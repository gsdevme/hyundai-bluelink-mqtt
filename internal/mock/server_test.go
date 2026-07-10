package mock_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/mock"
)

// newClient wires a real bluelink.Client at the mock server.
func newClient(t *testing.T, srv *httptest.Server, store bluelink.TokenStore) *bluelink.Client {
	t.Helper()
	if store == nil {
		store = bluelink.NewMemoryStore()
	}
	c, err := bluelink.New(bluelink.Config{
		Username:  "user@example.com",
		Password:  "secret",
		LoginHost: srv.URL,
		SPABase:   srv.URL,
		Store:     store,
		Now:       time.Now,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func TestClientAgainstMock_FullFlow(t *testing.T) {
	m, err := mock.New(mock.Defaults())
	if err != nil {
		t.Fatalf("mock: %v", err)
	}
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	c := newClient(t, srv, nil)
	ctx := t.Context()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	v, err := c.SelectVehicle(ctx)
	if err != nil {
		t.Fatalf("select vehicle: %v", err)
	}
	if v.VIN != "REDACTEDVIN000001" || !v.IsCCS2() {
		t.Fatalf("unexpected vehicle %+v", v)
	}

	st, err := c.CachedStatus(ctx, v)
	if err != nil {
		t.Fatalf("cached status: %v", err)
	}
	if st.EVBatteryPercentage == nil || *st.EVBatteryPercentage != 62 {
		t.Fatalf("battery = %v, want 62", st.EVBatteryPercentage)
	}
	if st.Charging == nil || !*st.Charging {
		t.Fatal("expected charging")
	}
	if st.Latitude == nil || st.LocationUpdatedAt == nil {
		t.Fatal("expected location from park endpoint")
	}

	cached, force, register, grants := m.Counts()
	if cached != 1 || force != 0 || register != 1 {
		t.Fatalf("counts cached=%d force=%d register=%d", cached, force, register)
	}
	if len(grants) != 1 || grants[0] != "authorization_code" {
		t.Fatalf("grants = %v, want [authorization_code]", grants)
	}
}

func TestClientAgainstMock_TokenRefresh(t *testing.T) {
	m, err := mock.New(mock.Defaults())
	if err != nil {
		t.Fatalf("mock: %v", err)
	}
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	// Preload the store with an expired access token but a valid refresh token
	// and device id: Connect must refresh rather than log in afresh.
	store := bluelink.NewMemoryStore()
	_ = store.Save(t.Context(), bluelink.Tokens{
		AccessToken:  "Bearer stale",
		RefreshToken: "mock-refresh-preloaded",
		DeviceID:     "mock-device-preloaded",
		ValidUntil:   time.Now().Add(-time.Hour),
	})

	c := newClient(t, srv, store)
	if err := c.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	_, _, register, grants := m.Counts()
	if register != 0 {
		t.Fatalf("expected no device registration on refresh path, got %d", register)
	}
	if len(grants) != 1 || grants[0] != "refresh_token" {
		t.Fatalf("grants = %v, want [refresh_token]", grants)
	}
}

func TestClientAgainstMock_ForceRefresh(t *testing.T) {
	m, err := mock.New(mock.Defaults())
	if err != nil {
		t.Fatalf("mock: %v", err)
	}
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	c := newClient(t, srv, nil)
	ctx := t.Context()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	v, _ := c.SelectVehicle(ctx)
	st, err := c.ForceStatus(ctx, v)
	if err != nil {
		t.Fatalf("force status: %v", err)
	}
	// Forced read reports battery+1 (63) vs cached 62.
	if st.EVBatteryPercentage == nil || *st.EVBatteryPercentage != 63 {
		t.Fatalf("forced battery = %v, want 63", st.EVBatteryPercentage)
	}
	_, force, _, _ := m.Counts()
	if force != 1 {
		t.Fatalf("force hits = %d, want 1", force)
	}
}
