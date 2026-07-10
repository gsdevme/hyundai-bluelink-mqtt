package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/homeassistant"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/mock"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/publisher"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/scheduler"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/server"
)

// world holds per-scenario state.
type world struct {
	opts           mock.Options
	forceGated     bool
	preloadExpired bool

	mock    *mock.Server
	srv     *httptest.Server
	store   bluelink.TokenStore
	client  *bluelink.Client
	rec     *publisher.RecordingPublisher
	pub     *publisher.Service
	status  *server.Server
	sched   *scheduler.Scheduler
	haCfg   homeassistant.Config
	vehicle bluelink.Vehicle
	ctx     context.Context
}

func (w *world) reset() {
	w.opts = mock.Defaults()
	w.forceGated = false
	w.preloadExpired = false
	w.mock = nil
	w.srv = nil
	w.store = bluelink.NewMemoryStore()
	w.ctx = context.Background()
}

func (w *world) cleanup() {
	if w.srv != nil {
		w.srv.Close()
	}
}

// --- Given ---

func (w *world) chargingPluggedInster() error {
	w.opts = mock.Defaults() // already charging + plugged
	return nil
}

func (w *world) insterUnplugged() error {
	w.opts.PluggedIn = false
	w.opts.ChargeRemainMin = 0
	return nil
}

func (w *world) insterCCS1() error {
	w.opts.CCS2 = false
	return nil
}

func (w *world) forceGatedOnPluggedIn() error {
	w.forceGated = true
	return nil
}

func (w *world) storedExpiredToken() error {
	w.preloadExpired = true
	return nil
}

// --- When ---

func (w *world) serviceStartsUp() error {
	m, err := mock.New(w.opts)
	if err != nil {
		return err
	}
	w.mock = m
	w.srv = httptest.NewServer(m.Handler())

	if w.preloadExpired {
		if err := w.store.Save(w.ctx, bluelink.Tokens{
			AccessToken:  "Bearer stale",
			RefreshToken: "mock-refresh-preloaded",
			DeviceID:     "mock-device-preloaded",
			ValidUntil:   time.Now().Add(-time.Hour),
		}); err != nil {
			return err
		}
	}

	client, err := bluelink.New(bluelink.Config{
		Username:  "user@example.com",
		Password:  "secret",
		LoginHost: w.srv.URL,
		SPABase:   w.srv.URL,
		Store:     w.store,
	})
	if err != nil {
		return err
	}
	w.client = client
	if err := client.Connect(w.ctx); err != nil {
		return err
	}
	v, err := client.SelectVehicle(w.ctx)
	if err != nil {
		return err
	}
	w.vehicle = v

	w.haCfg = homeassistant.Config{
		DiscoveryPrefix: "homeassistant",
		TopicPrefix:     "hyundai_bluelink",
		VIN:             v.VIN,
		Model:           v.Model,
		Name:            v.Name,
	}
	w.rec = publisher.NewRecordingPublisher()
	w.pub = publisher.New(w.rec, w.haCfg)
	w.status = server.New(server.Config{ReadyFailureThreshold: 3})

	if err := w.pub.PublishDiscovery(w.ctx); err != nil {
		return err
	}
	if err := w.pub.PublishAvailability(w.ctx, true); err != nil {
		return err
	}

	w.sched = scheduler.New(w.client, w.pub, w.status, scheduler.Config{
		Vehicle:                v,
		PollInterval:           1000 * time.Hour,
		MaxRetries:             0, // fail fast in acceptance tests
		ForceLocation:          time.UTC,
		ForceOnlyWhenPluggedIn: w.forceGated,
	})
	return nil
}

func (w *world) cachedPollRuns() error {
	w.sched.PollNow(w.ctx)
	return nil
}

func (w *world) nCachedPollsRun(n int) error {
	for range n {
		w.sched.PollNow(w.ctx)
	}
	return nil
}

func (w *world) scheduledForceRefreshRuns() error {
	w.sched.ForceNow(w.ctx)
	return nil
}

func (w *world) apiStartsFailing() error {
	w.mock.SetFailStatus(true)
	return nil
}

func (w *world) serviceShutsDown() error {
	return w.pub.PublishAvailability(w.ctx, false)
}

// --- Then ---

func (w *world) discoveryPublishedRetained(component, key string) error {
	topic := fmt.Sprintf("homeassistant/%s/%s_%s/config", component, w.vehicle.VIN, key)
	rec, ok := w.rec.Get(topic)
	if !ok {
		return fmt.Errorf("discovery topic %s not published", topic)
	}
	if !rec.Retain {
		return fmt.Errorf("discovery topic %s not retained", topic)
	}
	return nil
}

func (w *world) batteryDiscovery() error {
	return w.discoveryPublishedRetained("sensor", "ev_battery_percentage")
}
func (w *world) chargingDiscovery() error {
	return w.discoveryPublishedRetained("binary_sensor", "charging")
}
func (w *world) trackerDiscovery() error {
	return w.discoveryPublishedRetained("device_tracker", "location")
}

func (w *world) availabilityPublished(payload string) error {
	rec, ok := w.rec.Get(w.haCfg.AvailabilityTopic())
	if !ok {
		return fmt.Errorf("availability not published")
	}
	if string(rec.Payload) != payload {
		return fmt.Errorf("availability = %q, want %q", rec.Payload, payload)
	}
	if !rec.Retain {
		return fmt.Errorf("availability not retained")
	}
	return nil
}

func (w *world) stateField(key string) (any, error) {
	rec, ok := w.rec.Get(w.haCfg.StateTopic())
	if !ok {
		return nil, fmt.Errorf("state topic not published")
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Payload, &m); err != nil {
		return nil, err
	}
	return m[key], nil
}

func (w *world) stateReportsBattery(want int) error {
	v, err := w.stateField("ev_battery_percentage")
	if err != nil {
		return err
	}
	f, ok := v.(float64)
	if !ok || int(f) != want {
		return fmt.Errorf("battery = %v, want %d", v, want)
	}
	return nil
}

func (w *world) stateReportsBool(key string, want bool) error {
	v, err := w.stateField(key)
	if err != nil {
		return err
	}
	if v != want {
		return fmt.Errorf("%s = %v, want %v", key, v, want)
	}
	return nil
}

func (w *world) tokenGrantReceived(grant string) error {
	_, _, _, grants := w.mock.Counts()
	if !slices.Contains(grants, grant) {
		return fmt.Errorf("grants %v do not include %q", grants, grant)
	}
	return nil
}

func (w *world) noDeviceRegistration() error {
	_, _, register, _ := w.mock.Counts()
	if register != 0 {
		return fmt.Errorf("expected no device registration, got %d", register)
	}
	return nil
}

func (w *world) forceCalled(want bool) error {
	_, force, _, _ := w.mock.Counts()
	if want && force == 0 {
		return fmt.Errorf("force endpoint was not called")
	}
	if !want && force != 0 {
		return fmt.Errorf("force endpoint was called %d times, expected none", force)
	}
	return nil
}

func (w *world) readiness(ready bool) error {
	if w.status.Ready() != ready {
		return fmt.Errorf("readiness = %v, want %v", w.status.Ready(), ready)
	}
	return nil
}

func TestFeatures(t *testing.T) {
	w := &world{}
	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			ctx.Before(func(c context.Context, _ *godog.Scenario) (context.Context, error) {
				w.reset()
				return c, nil
			})
			ctx.After(func(c context.Context, _ *godog.Scenario, err error) (context.Context, error) {
				w.cleanup()
				return c, err
			})

			ctx.Step(`^a charging, plugged-in Inster on the Bluelink account$`, w.chargingPluggedInster)
			ctx.Step(`^the Inster is unplugged$`, w.insterUnplugged)
			ctx.Step(`^the Inster speaks the CCS1 protocol$`, w.insterCCS1)
			ctx.Step(`^force refresh is gated on being plugged in$`, w.forceGatedOnPluggedIn)
			ctx.Step(`^a stored but expired access token with a valid refresh token$`, w.storedExpiredToken)

			ctx.Step(`^the service starts up$`, w.serviceStartsUp)
			ctx.Step(`^a cached poll runs$`, w.cachedPollRuns)
			ctx.Step(`^(\d+) cached polls run$`, w.nCachedPollsRun)
			ctx.Step(`^a scheduled force refresh runs$`, w.scheduledForceRefreshRuns)
			ctx.Step(`^the Bluelink API starts failing$`, w.apiStartsFailing)
			ctx.Step(`^the service shuts down$`, w.serviceShutsDown)

			ctx.Step(`^the battery sensor discovery config is published retained$`, w.batteryDiscovery)
			ctx.Step(`^the charging binary_sensor discovery config is published retained$`, w.chargingDiscovery)
			ctx.Step(`^the device_tracker discovery config is published retained$`, w.trackerDiscovery)
			ctx.Step(`^availability "([^"]*)" is published retained$`, w.availabilityPublished)

			ctx.Step(`^the state topic reports battery (\d+)$`, w.stateReportsBattery)
			ctx.Step(`^the state topic still reports battery (\d+)$`, w.stateReportsBattery)
			ctx.Step(`^the state topic reports charging (true|false)$`, func(s string) error {
				return w.stateReportsBool("charging", s == "true")
			})
			ctx.Step(`^the state topic reports plugged_in (true|false)$`, func(s string) error {
				return w.stateReportsBool("plugged_in", s == "true")
			})

			ctx.Step(`^the token endpoint received a "([^"]*)" grant$`, w.tokenGrantReceived)
			ctx.Step(`^no device registration occurred$`, w.noDeviceRegistration)
			ctx.Step(`^the force status endpoint was called$`, func() error { return w.forceCalled(true) })
			ctx.Step(`^the force status endpoint was not called$`, func() error { return w.forceCalled(false) })
			ctx.Step(`^the readiness endpoint reports ready$`, func() error { return w.readiness(true) })
			ctx.Step(`^the readiness endpoint reports not ready$`, func() error { return w.readiness(false) })
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"."},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("godog acceptance suite failed")
	}
}
