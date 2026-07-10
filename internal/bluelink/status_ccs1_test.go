package bluelink

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/mock"
)

// TestCCS1StatusAgainstMock exercises the CachedStatus/ForceStatus client paths
// against the mock's CCS1 endpoints (selected by Options.CCS2 = false), proving
// the protocol is chosen from the vehicle's reported support.
func TestCCS1StatusAgainstMock(t *testing.T) {
	opts := mock.Defaults()
	opts.CCS2 = false // report ccuCCS2ProtocolSupport=0 => CCS1

	m, err := mock.New(opts)
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	client, err := New(Config{
		Username:  "user@example.com",
		Password:  "secret",
		LoginHost: srv.URL,
		SPABase:   srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	v, err := client.SelectVehicle(ctx)
	if err != nil {
		t.Fatalf("SelectVehicle: %v", err)
	}
	if v.IsCCS2() {
		t.Fatal("expected a CCS1 vehicle (IsCCS2 == false)")
	}

	cached, err := client.CachedStatus(ctx, v)
	if err != nil {
		t.Fatalf("CachedStatus: %v", err)
	}
	if got := f64(t, cached.EVBatteryPercentage); got != opts.BatteryPercent {
		t.Errorf("cached battery = %v, want %v", got, opts.BatteryPercent)
	}
	if cached.Charging == nil || !*cached.Charging {
		t.Error("expected charging = true (ChargeRemainMin > 0)")
	}
	if cached.PluggedIn == nil || !*cached.PluggedIn {
		t.Error("expected plugged in = true")
	}
	if got := f64(t, cached.Latitude); got != opts.Latitude {
		t.Errorf("latitude = %v, want %v (from embedded vehicleLocation)", got, opts.Latitude)
	}

	forced, err := client.ForceStatus(ctx, v)
	if err != nil {
		t.Fatalf("ForceStatus: %v", err)
	}
	if got := f64(t, forced.EVBatteryPercentage); got != opts.BatteryPercent+1 {
		t.Errorf("forced battery = %v, want %v", got, opts.BatteryPercent+1)
	}

	cachedHits, forceHits, _, _ := m.Counts()
	if cachedHits == 0 {
		t.Error("expected the CCS1 cached endpoint to be hit")
	}
	if forceHits == 0 {
		t.Error("expected the CCS1 force endpoint to be hit")
	}
}
