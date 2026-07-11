package cmd

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/server"
)

func ptr[T any](v T) *T { return &v }

// fakePublisher records the last state and optionally fails.
type fakePublisher struct {
	err    error
	called bool
	last   bluelink.VehicleState
}

func (f *fakePublisher) PublishState(_ context.Context, st bluelink.VehicleState) error {
	f.called = true
	f.last = st
	if f.err != nil {
		return f.err
	}
	return nil
}

func TestMetricsFromState(t *testing.T) {
	updated := time.Date(2026, 7, 11, 8, 30, 0, 0, time.UTC)
	st := bluelink.VehicleState{
		EVBatteryPercentage:      ptr(82.0),
		EVBatterySoH:             ptr(99.5),
		EVRange:                  ptr(240.0),
		EVRangeUnit:              "km",
		Charging:                 ptr(true),
		PluggedIn:                ptr(true),
		ChargePortDoorOpen:       ptr(false),
		ChargeLimitAC:            ptr(80.0),
		ChargeLimitDC:            ptr(100.0),
		ChargingPowerKW:          ptr(7.4),
		EstChargeDurationMin:     ptr(120),
		EstFastChargeDurationMin: ptr(35),
		Battery12VPercentage:     ptr(90),
		OutsideTemperatureC:      ptr(18.5),
		InsideTemperatureC:       ptr(21.0),
		TirePressureWarning:      ptr(false),
		LastUpdatedAt:            &updated,
		// Excluded (personal/sensitive) fields — must not surface in Metrics.
		Odometer:          ptr(12345.0),
		OdometerUnit:      "km",
		Locked:            ptr(true),
		Latitude:          ptr(51.5),
		Longitude:         ptr(-0.12),
		LocationUpdatedAt: &updated,
	}

	got := metricsFromState(st)

	want := server.Metrics{
		EVBatteryPercentage:      st.EVBatteryPercentage,
		EVBatterySoH:             st.EVBatterySoH,
		EVRange:                  st.EVRange,
		EVRangeUnit:              "km",
		Charging:                 st.Charging,
		PluggedIn:                st.PluggedIn,
		ChargePortDoorOpen:       st.ChargePortDoorOpen,
		ChargeLimitAC:            st.ChargeLimitAC,
		ChargeLimitDC:            st.ChargeLimitDC,
		ChargingPowerKW:          st.ChargingPowerKW,
		EstChargeDurationMin:     st.EstChargeDurationMin,
		EstFastChargeDurationMin: st.EstFastChargeDurationMin,
		Battery12VPercentage:     st.Battery12VPercentage,
		OutsideTemperatureC:      st.OutsideTemperatureC,
		InsideTemperatureC:       st.InsideTemperatureC,
		TirePressureWarning:      st.TirePressureWarning,
		LastUpdatedAt:            st.LastUpdatedAt,
	}

	if got != want {
		t.Errorf("metricsFromState mismatch:\n got %+v\nwant %+v", got, want)
	}
	// Metrics has no location/odometer/lock fields, so mapping cannot leak them;
	// this is a compile-time guarantee. The value equality above covers the rest.
}

// pageHasMetrics reports whether the / page renders a Metrics section.
func pageHasMetrics(t *testing.T, s *server.Server) bool {
	t.Helper()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	return strings.Contains(string(body), "<h2>Metrics</h2>")
}

func TestRecordingPublisher(t *testing.T) {
	t.Run("success records snapshot", func(t *testing.T) {
		fake := &fakePublisher{}
		status := server.New(server.Config{ReadyFailureThreshold: 1})
		rp := recordingPublisher{pub: fake, status: status}

		if err := rp.PublishState(context.Background(), bluelink.VehicleState{EVBatteryPercentage: ptr(50.0)}); err != nil {
			t.Fatalf("PublishState: %v", err)
		}
		if !fake.called {
			t.Fatal("underlying publisher not called")
		}
		if !pageHasMetrics(t, status) {
			t.Fatal("snapshot not recorded after successful publish")
		}
	})

	t.Run("failure propagates and does not record", func(t *testing.T) {
		wantErr := errors.New("boom")
		fake := &fakePublisher{err: wantErr}
		status := server.New(server.Config{ReadyFailureThreshold: 1})
		rp := recordingPublisher{pub: fake, status: status}

		err := rp.PublishState(context.Background(), bluelink.VehicleState{EVBatteryPercentage: ptr(50.0)})
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want %v", err, wantErr)
		}
		if pageHasMetrics(t, status) {
			t.Fatal("snapshot recorded despite publish failure (should keep last good)")
		}
	})
}
