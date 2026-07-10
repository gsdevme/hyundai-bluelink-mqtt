package bluelink

import (
	"encoding/json"
	"os"
	"testing"
)

// loadVehicleState reads a full status fixture and parses resMsg.state.Vehicle.
func loadVehicleState(t *testing.T, path string) VehicleState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var env struct {
		ResMsg struct {
			State struct {
				Vehicle map[string]any `json:"Vehicle"`
			} `json:"state"`
		} `json:"resMsg"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if env.ResMsg.State.Vehicle == nil {
		t.Fatal("fixture missing resMsg.state.Vehicle")
	}
	return parseCCS2(env.ResMsg.State.Vehicle)
}

func f64(t *testing.T, p *float64) float64 {
	t.Helper()
	if p == nil {
		t.Fatal("expected non-nil float")
	}
	return *p
}

func TestParseCCS2Inster(t *testing.T) {
	s := loadVehicleState(t, "testdata/ccs2_carstatus_latest.json")

	if got := f64(t, s.EVBatteryPercentage); got != 62.0 {
		t.Errorf("battery = %v, want 62", got)
	}
	if got := f64(t, s.EVBatterySoH); got != 99.0 {
		t.Errorf("soh = %v, want 99", got)
	}
	if got := f64(t, s.EVRange); got != 268.0 {
		t.Errorf("range = %v, want 268", got)
	}
	if s.EVRangeUnit != "km" {
		t.Errorf("range unit = %q, want km", s.EVRangeUnit)
	}
	if s.Charging == nil || !*s.Charging {
		t.Error("expected charging = true (RemainTime 145 > 0)")
	}
	if s.EstChargeDurationMin == nil || *s.EstChargeDurationMin != 145 {
		t.Errorf("est charge duration = %v, want 145", s.EstChargeDurationMin)
	}
	if s.EstFastChargeDurationMin == nil || *s.EstFastChargeDurationMin != 42 {
		t.Errorf("fast charge duration = %v, want 42", s.EstFastChargeDurationMin)
	}
	if s.PluggedIn == nil || !*s.PluggedIn {
		t.Error("expected plugged in = true")
	}
	if s.ChargePortDoorOpen == nil || !*s.ChargePortDoorOpen {
		t.Error("expected charge port door open = true")
	}
	if got := f64(t, s.ChargeLimitAC); got != 80 {
		t.Errorf("AC limit = %v, want 80", got)
	}
	if got := f64(t, s.ChargeLimitDC); got != 100 {
		t.Errorf("DC limit = %v, want 100", got)
	}
	if got := f64(t, s.ChargingPowerKW); got != 7.2 {
		t.Errorf("power = %v, want 7.2", got)
	}
	if s.Battery12VPercentage == nil || *s.Battery12VPercentage != 87 {
		t.Errorf("12V = %v, want 87", s.Battery12VPercentage)
	}
	if got := f64(t, s.Odometer); got != 4213.5 {
		t.Errorf("odometer = %v, want 4213.5", got)
	}
	if s.OdometerUnit != "km" {
		t.Errorf("odometer unit = %q, want km", s.OdometerUnit)
	}
	if got := f64(t, s.OutsideTemperatureC); got != 18.5 {
		t.Errorf("outside temp = %v, want 18.5", got)
	}
	if got := f64(t, s.InsideTemperatureC); got != 21.0 {
		t.Errorf("inside temp = %v, want 21", got)
	}
	if s.Locked == nil || !*s.Locked {
		t.Error("expected locked = true (all door Lock=0 => locked)")
	}
	if s.TirePressureWarning == nil || *s.TirePressureWarning {
		t.Error("expected tire warning = false")
	}
	if s.LastUpdatedAt == nil {
		t.Error("expected LastUpdatedAt to be parsed")
	}
}

func TestNormalizeBatterySoC(t *testing.T) {
	fp := func(v float64) *float64 { return &v }
	ip := func(v int) *int { return &v }
	tests := []struct {
		name        string
		value       *float64
		reliability *int
		want        *int
	}{
		{"valid", fp(87), ip(0), ip(87)},
		{"unreliable", fp(87), ip(1), nil},
		{"sentinel 255", fp(255), ip(0), nil},
		{"negative", fp(-1), nil, nil},
		{"missing", nil, nil, nil},
		{"boundary 100", fp(100), nil, ip(100)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeBatterySoC(tc.value, tc.reliability)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("got %v, want nil", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("got nil, want %v", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("got %v, want %v", *got, *tc.want)
			}
		})
	}
}

func TestTemperatureFahrenheitConversion(t *testing.T) {
	v := 68.0 // °F
	unit := 1
	got := temperatureC(&v, &unit)
	if got == nil || *got != 20.0 {
		t.Fatalf("68°F = %v, want 20°C", got)
	}
}

func TestGetPathAbsentIsNil(t *testing.T) {
	m := map[string]any{"a": map[string]any{"b": 1.0}}
	if getPath(m, "a.b.c.d") != nil {
		t.Error("expected nil for path descending past a leaf")
	}
	if getFloat(m, "x.y.z") != nil {
		t.Error("expected nil float for absent path")
	}
}

func TestForceFixtureDiffers(t *testing.T) {
	cached := loadVehicleState(t, "testdata/ccs2_carstatus_latest.json")
	forced := loadVehicleState(t, "testdata/ccs2_carstatus_force.json")
	if f64(t, cached.EVBatteryPercentage) == f64(t, forced.EVBatteryPercentage) {
		t.Error("force fixture should report a different battery % to prove refresh took effect")
	}
}
