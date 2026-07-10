package bluelink

import (
	"strings"
	"time"
)

// parseCCS2 maps a CCS2 status document (the object at resMsg.state.Vehicle) into
// the normalised VehicleState. Absent fields leave the corresponding pointer nil.
func parseCCS2(state map[string]any) VehicleState {
	var s VehicleState

	s.EVBatteryPercentage = getFloat(state, "Green.BatteryManagement.BatteryRemain.Ratio")
	s.EVBatterySoH = getFloat(state, "Green.BatteryManagement.SoH.Ratio")

	// Range + unit (distance index -> km/mi).
	s.EVRange = getFloat(state, "Drivetrain.FuelSystem.DTE.Total")
	s.EVRangeUnit = distanceUnit(getInt(state, "Drivetrain.FuelSystem.DTE.Unit"))

	// Charging is derived from the remaining charge time (reference behaviour):
	// 0 -> not charging, >0 -> charging. Absent -> unknown.
	if remain := getInt(state, "Green.ChargingInformation.Charging.RemainTime"); remain != nil {
		charging := *remain > 0
		s.Charging = &charging
		s.EstChargeDurationMin = remain
	}
	s.EstFastChargeDurationMin = getInt(state, "Green.ChargingInformation.EstimatedTime.Quick")

	// Plugged in from the connector-fastening flag.
	if v := getInt(state, "Green.ChargingInformation.ConnectorFastening.State"); v != nil {
		plugged := *v != 0
		s.PluggedIn = &plugged
	}

	// Charge-port door: 1 open; 0/2 closed; else unknown.
	if v := getInt(state, "Green.ChargingDoor.State"); v != nil {
		switch *v {
		case 1:
			s.ChargePortDoorOpen = boolPtr(true)
		case 0, 2:
			s.ChargePortDoorOpen = boolPtr(false)
		}
	}

	s.ChargeLimitAC = getFloat(state, "Green.ChargingInformation.TargetSoC.Standard")
	s.ChargeLimitDC = getFloat(state, "Green.ChargingInformation.TargetSoC.Quick")
	s.ChargingPowerKW = getFloat(state, "Green.Electric.SmartGrid.RealTimePower")

	// 12V auxiliary battery, normalised (reliability flag + range guard).
	s.Battery12VPercentage = normalizeBatterySoC(
		getFloat(state, "Electronics.Battery.Level"),
		getInt(state, "Electronics.Battery.SensorReliability"),
	)

	s.Odometer = getFloat(state, "Drivetrain.Odometer")
	if s.Odometer != nil {
		s.OdometerUnit = "km" // DISTANCE_UNITS[1]
	}

	s.OutsideTemperatureC = temperatureC(
		getFloat(state, "Cabin.HVAC.OutsideTemperature.Value"),
		getInt(state, "Cabin.HVAC.OutsideTemperature.Unit"),
	)
	// Inside temperature is unset ("OFF") when the HVAC is off.
	if raw := getPath(state, "Cabin.HVAC.Row1.Driver.Temperature.Value"); raw != nil {
		if f, ok := asFloat(raw); ok {
			s.InsideTemperatureC = temperatureC(&f, getInt(state, "Cabin.HVAC.Row1.Driver.Temperature.Unit"))
		}
	}

	s.Locked = allDoorsLocked(state)
	s.TirePressureWarning = tireWarning(state)

	if t, ok := parseCCS2Date(getString(state, "Date")); ok {
		s.LastUpdatedAt = &t
	}
	return s
}

// allDoorsLocked reports whether every present door is locked. In CCS2 the "Lock"
// field is truthy when the door is *unlocked*, so locked = !truthy. Returns nil
// when any door's state is absent.
func allDoorsLocked(state map[string]any) *bool {
	paths := []string{
		"Cabin.Door.Row1.Driver.Lock",
		"Cabin.Door.Row1.Passenger.Lock",
		"Cabin.Door.Row2.Left.Lock",
		"Cabin.Door.Row2.Right.Lock",
	}
	locked := true
	seen := 0
	for _, p := range paths {
		v := getInt(state, p)
		if v == nil {
			return nil
		}
		seen++
		if *v != 0 { // truthy => unlocked
			locked = false
		}
	}
	if seen == 0 {
		return nil
	}
	return &locked
}

// tireWarning ORs the aggregate and per-axle low-pressure flags. Returns nil when
// none are present.
func tireWarning(state map[string]any) *bool {
	paths := []string{
		"Chassis.Axle.Tire.PressureLow",
		"Chassis.Axle.Row1.Left.Tire.PressureLow",
		"Chassis.Axle.Row1.Right.Tire.PressureLow",
		"Chassis.Axle.Row2.Left.Tire.PressureLow",
		"Chassis.Axle.Row2.Right.Tire.PressureLow",
	}
	warn := false
	seen := false
	for _, p := range paths {
		v := getInt(state, p)
		if v == nil {
			continue
		}
		seen = true
		if *v != 0 {
			warn = true
		}
	}
	if !seen {
		return nil
	}
	return &warn
}

// normalizeBatterySoC normalises the 12V SoC: nil when unreliable, missing, or
// out of the 0..100 range (e.g. the 255/0xFF sentinel).
func normalizeBatterySoC(value *float64, reliability *int) *int {
	if reliability != nil && *reliability == 1 {
		return nil
	}
	if value == nil {
		return nil
	}
	if *value < 0 || *value > 100 {
		return nil
	}
	v := int(*value)
	return &v
}

// distanceUnit maps a CCS2 distance-unit index to a unit string.
func distanceUnit(idx *int) string {
	if idx == nil {
		return ""
	}
	switch *idx {
	case 1:
		return "km"
	case 2, 3:
		return "mi"
	default:
		return ""
	}
}

// temperatureC returns the temperature in °C, converting from °F when the unit
// index is 1. Returns nil when the value is nil.
func temperatureC(value *float64, unitIdx *int) *float64 {
	if value == nil {
		return nil
	}
	c := *value
	if unitIdx != nil && *unitIdx == 1 { // Fahrenheit
		c = (c - 32) * 5 / 9
	}
	return &c
}

// parseCCS2Date parses the CCS2 "Date" field (UTC).
func parseCCS2Date(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "20060102150405", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// --- safe dotted-path lookups (never panic on absent keys) ---

// getPath walks a dotted path through nested maps, returning nil if any segment
// is missing or not a map.
func getPath(m map[string]any, path string) any {
	var cur any = m
	for _, seg := range strings.Split(path, ".") {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = asMap[seg]
		if !ok {
			return nil
		}
	}
	return cur
}

func getFloat(m map[string]any, path string) *float64 {
	if f, ok := asFloat(getPath(m, path)); ok {
		return &f
	}
	return nil
}

func getInt(m map[string]any, path string) *int {
	if f, ok := asFloat(getPath(m, path)); ok {
		i := int(f)
		return &i
	}
	return nil
}

func getString(m map[string]any, path string) string {
	if s, ok := getPath(m, path).(string); ok {
		return s
	}
	return ""
}

// asFloat coerces a JSON-decoded value to float64.
func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func boolPtr(b bool) *bool { return &b }
