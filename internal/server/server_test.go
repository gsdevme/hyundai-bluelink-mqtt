package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func TestReadinessLifecycle(t *testing.T) {
	s := New(Config{ReadyFailureThreshold: 3})
	if s.Ready() {
		t.Fatal("should start not ready")
	}
	s.MarkSuccess()
	if !s.Ready() {
		t.Fatal("should be ready after first success")
	}
	s.MarkFailure()
	s.MarkFailure()
	if !s.Ready() {
		t.Fatal("should stay ready below threshold")
	}
	s.MarkFailure() // 3rd consecutive
	if s.Ready() {
		t.Fatal("should be not ready at threshold")
	}
	s.MarkSuccess()
	if !s.Ready() {
		t.Fatal("should recover on success")
	}
}

func TestProbes(t *testing.T) {
	s := New(Config{ReadyFailureThreshold: 1})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Liveness always OK.
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %v / %v", resp, err)
	}
	// Readiness 503 before first success.
	resp, _ = http.Get(srv.URL + "/readyz")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz before success = %d, want 503", resp.StatusCode)
	}
	s.MarkSuccess()
	resp, _ = http.Get(srv.URL + "/readyz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz after success = %d, want 200", resp.StatusCode)
	}
}

func TestRootStatusPage(t *testing.T) {
	s := New(Config{ReadyFailureThreshold: 1})
	s.SetVehicle("INSTER", "Inster", "REDACTEDVIN000001", true)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	page := string(body)
	if !strings.Contains(page, "not ready") {
		t.Fatalf("page missing readiness status:\n%s", page)
	}
	if !strings.Contains(page, maskVIN("REDACTEDVIN000001")) {
		t.Fatalf("page missing masked VIN:\n%s", page)
	}
	if strings.Contains(page, "REDACTEDVIN000001") {
		t.Fatalf("page leaked the full VIN:\n%s", page)
	}
}

// personalMarkers must never appear on the / page — it shares HEALTH_ADDR with
// the probes and may be reachable by others.
var personalMarkers = []string{"latitude", "longitude", "odometer", "Locked", "locked", "Lock"}

func TestRootMetricsSection(t *testing.T) {
	updated := time.Date(2026, 7, 11, 8, 30, 0, 0, time.UTC)
	full := Metrics{
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
	}

	tests := []struct {
		name        string
		set         bool
		metrics     Metrics
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:       "before SetMetrics no section",
			set:        false,
			wantAbsent: []string{"<h2>Metrics</h2>", "Battery:"},
		},
		{
			name:    "fully populated",
			set:     true,
			metrics: full,
			wantContain: []string{
				"<h2>Metrics</h2>",
				"Battery: 82 %",
				"State of health: 99.5 %",
				"Range: 240 km",
				"Charging: yes",
				"Plugged in: yes",
				"Charge port door: no",
				"Charge limit (AC): 80 %",
				"Charge limit (DC): 100 %",
				"Charging power: 7.4 kW",
				"Est. charge time: 120 min",
				"Est. fast-charge time: 35 min",
				"12V battery: 90 %",
				"Outside temperature: 18.5 °C",
				"Inside temperature: 21 °C",
				"Tyre pressure warning: no",
				"Last updated: 2026-07-11T08:30:00Z",
			},
		},
		{
			name: "partially nil renders unknown",
			set:  true,
			metrics: Metrics{
				EVBatteryPercentage: ptr(50.0),
				// EVRange nil with a unit set must not render "unknown km".
				EVRangeUnit: "km",
			},
			wantContain: []string{
				"Battery: 50 %",
				"Range: unknown",
				"State of health: unknown",
				"Charging: unknown",
				"Last updated: unknown",
			},
			wantAbsent: []string{"unknown km"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(Config{ReadyFailureThreshold: 1})
			s.SetVehicle("INSTER", "Inster", "REDACTEDVIN000001", true)
			if tt.set {
				s.SetMetrics(tt.metrics)
			}
			srv := httptest.NewServer(s.Handler())
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/")
			if err != nil {
				t.Fatalf("GET / error: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET / = %d, want 200", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			page := string(body)

			for _, want := range tt.wantContain {
				if !strings.Contains(page, want) {
					t.Errorf("page missing %q:\n%s", want, page)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(page, absent) {
					t.Errorf("page unexpectedly contains %q:\n%s", absent, page)
				}
			}
			// Privacy guard: personal fields must never appear.
			for _, marker := range personalMarkers {
				if strings.Contains(page, marker) {
					t.Errorf("page leaked personal marker %q:\n%s", marker, page)
				}
			}
		})
	}
}

func TestUnknownPath404(t *testing.T) {
	s := New(Config{ReadyFailureThreshold: 1})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatalf("GET /nope error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /nope = %d, want 404", resp.StatusCode)
	}
}
