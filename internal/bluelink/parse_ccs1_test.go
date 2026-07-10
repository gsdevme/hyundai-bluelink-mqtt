package bluelink

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// loadVehicleStateCCS1 reads a CCS1 status fixture and parses
// resMsg.vehicleStatusInfo (mirrors loadVehicleState for CCS2).
func loadVehicleStateCCS1(t *testing.T, path string) VehicleState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var env struct {
		ResMsg struct {
			VehicleStatusInfo map[string]any `json:"vehicleStatusInfo"`
		} `json:"resMsg"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if env.ResMsg.VehicleStatusInfo == nil {
		t.Fatal("fixture missing resMsg.vehicleStatusInfo")
	}
	s, err := parseCCS1(env.ResMsg.VehicleStatusInfo)
	if err != nil {
		t.Fatalf("parseCCS1: %v", err)
	}
	return s
}

func TestParseCCS1Inster(t *testing.T) {
	s := loadVehicleStateCCS1(t, "testdata/ccs1_status_latest.json")

	if got := f64(t, s.EVBatteryPercentage); got != 59.0 {
		t.Errorf("battery = %v, want 59", got)
	}
	if s.EVBatterySoH != nil {
		t.Errorf("SoH = %v, want nil (CCS1 does not report it)", *s.EVBatterySoH)
	}
	if got := f64(t, s.EVRange); math.Abs(got-122.407767) > 0.001 {
		t.Errorf("range = %v, want ~122.41", got)
	}
	if s.EVRangeUnit != "mi" {
		t.Errorf("range unit = %q, want mi (unit code 3)", s.EVRangeUnit)
	}
	if s.Charging == nil || *s.Charging {
		t.Error("expected charging = false (batteryCharge false)")
	}
	if s.PluggedIn == nil || *s.PluggedIn {
		t.Error("expected plugged in = false (batteryPlugin 0)")
	}
	if s.ChargePortDoorOpen != nil {
		t.Errorf("charge port door = %v, want nil (CCS1 does not report it)", *s.ChargePortDoorOpen)
	}
	if got := f64(t, s.ChargeLimitAC); got != 80 {
		t.Errorf("AC limit = %v, want 80 (plugType 1)", got)
	}
	if got := f64(t, s.ChargeLimitDC); got != 100 {
		t.Errorf("DC limit = %v, want 100 (plugType 0)", got)
	}
	if s.ChargingPowerKW != nil {
		t.Errorf("power = %v, want nil (CCS1 does not report it)", *s.ChargingPowerKW)
	}
	if s.EstChargeDurationMin == nil || *s.EstChargeDurationMin != 60 {
		t.Errorf("est charge duration = %v, want 60 (atc)", s.EstChargeDurationMin)
	}
	if s.EstFastChargeDurationMin == nil || *s.EstFastChargeDurationMin != 60 {
		t.Errorf("fast charge duration = %v, want 60 (etc3)", s.EstFastChargeDurationMin)
	}
	if s.Battery12VPercentage == nil || *s.Battery12VPercentage != 94 {
		t.Errorf("12V = %v, want 94 (batSoc)", s.Battery12VPercentage)
	}
	if got := f64(t, s.Odometer); got != 144.3 {
		t.Errorf("odometer = %v, want 144.3", got)
	}
	if s.OdometerUnit != "km" {
		t.Errorf("odometer unit = %q, want km (unit code 1)", s.OdometerUnit)
	}
	if s.OutsideTemperatureC != nil {
		t.Errorf("outside temp = %v, want nil (CCS1 does not report it)", *s.OutsideTemperatureC)
	}
	if s.InsideTemperatureC != nil {
		t.Errorf("inside temp = %v, want nil (airTemp is a setpoint, not a reading)", *s.InsideTemperatureC)
	}
	if s.Locked == nil || !*s.Locked {
		t.Error("expected locked = true (doorLock true)")
	}
	if s.TirePressureWarning == nil || *s.TirePressureWarning {
		t.Error("expected tire warning = false (all lamps 0)")
	}
	if got := f64(t, s.Latitude); got != 51.5 {
		t.Errorf("latitude = %v, want 51.5", got)
	}
	if got := f64(t, s.Longitude); got != -0.12 {
		t.Errorf("longitude = %v, want -0.12", got)
	}
	if s.LocationUpdatedAt == nil {
		t.Error("expected LocationUpdatedAt to be parsed from vehicleLocation.time")
	}
	if s.LastUpdatedAt == nil {
		t.Error("expected LastUpdatedAt to be parsed from vehicleStatus.time")
	}
}

func TestParseCCS1MissingVehicleStatus(t *testing.T) {
	if _, err := parseCCS1(map[string]any{}); err == nil {
		t.Error("expected an error when vehicleStatus is absent")
	}
}
