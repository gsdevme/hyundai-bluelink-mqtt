package bluelink

import "errors"

// parseCCS1 maps a CCS1 status document (the object at resMsg.vehicleStatusInfo,
// which nests vehicleStatus, vehicleLocation and odometer) into the normalised
// VehicleState. It reuses the package-scoped path helpers from parse_ccs2.go so
// the two protocol parsers stay symmetric. Absent fields leave their pointer nil.
//
// CCS1 does not expose everything CCS2 does: battery state-of-health, the charge
// port door, real-time charging power and measured cabin/outside temperatures are
// all absent, so those fields stay nil (rendered "unknown" in Home Assistant).
func parseCCS1(state map[string]any) (VehicleState, error) {
	if getPath(state, "vehicleStatus") == nil {
		return VehicleState{}, errors.New("bluelink: CCS1 status missing vehicleStatus")
	}
	var s VehicleState

	s.EVBatteryPercentage = getFloat(state, "vehicleStatus.evStatus.batteryStatus")

	// Range + unit live in the first drvDistance entry (an array, so getPath can't
	// reach into it directly).
	if arr, ok := getPath(state, "vehicleStatus.evStatus.drvDistance").([]any); ok && len(arr) > 0 {
		if first, ok := arr[0].(map[string]any); ok {
			s.EVRange = getFloat(first, "rangeByFuel.totalAvailableRange.value")
			s.EVRangeUnit = distanceUnit(getInt(first, "rangeByFuel.totalAvailableRange.unit"))
		}
	}

	// CCS1 reports charging directly as a bool (unlike CCS2, which derives it).
	s.Charging = getBool(state, "vehicleStatus.evStatus.batteryCharge")

	// batteryPlugin: 0 none, 1 AC, 2 DC -> plugged when non-zero.
	if v := getInt(state, "vehicleStatus.evStatus.batteryPlugin"); v != nil {
		plugged := *v != 0
		s.PluggedIn = &plugged
	}

	// Target SoC per plug type: 0 => DC (fast), 1 => AC (slow).
	if arr, ok := getPath(state, "vehicleStatus.evStatus.reservChargeInfos.targetSOClist").([]any); ok {
		for _, e := range arr {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			level := getFloat(m, "targetSOClevel")
			plug := getInt(m, "plugType")
			if level == nil || plug == nil {
				continue
			}
			switch *plug {
			case 0:
				s.ChargeLimitDC = level
			case 1:
				s.ChargeLimitAC = level
			}
		}
	}

	// atc is the estimated time for the active/AC charge; etc3 is the DC (fast)
	// estimate. Both were captured while unplugged, so they are best-effort.
	s.EstChargeDurationMin = getInt(state, "vehicleStatus.evStatus.remainTime2.atc.value")
	s.EstFastChargeDurationMin = getInt(state, "vehicleStatus.evStatus.remainTime2.etc3.value")

	// 12V auxiliary battery. CCS1 has no reliability flag, so only the range guard
	// applies.
	s.Battery12VPercentage = normalizeBatterySoC(getFloat(state, "vehicleStatus.battery.batSoc"), nil)

	s.Odometer = getFloat(state, "odometer.value")
	s.OdometerUnit = distanceUnit(getInt(state, "odometer.unit"))

	// CCS1 only exposes a climate setpoint (vehicleStatus.airTemp, hex-encoded), not
	// a measured cabin or outside temperature, so both temperatures stay nil.

	s.Locked = getBool(state, "vehicleStatus.doorLock")
	s.TirePressureWarning = ccs1TireWarning(state)

	// Location is embedded in the status response (fresh, unlike CCS2's stale copy).
	if lat := getFloat(state, "vehicleLocation.coord.lat"); lat != nil {
		if lon := getFloat(state, "vehicleLocation.coord.lon"); lon != nil && (*lat != 0 || *lon != 0) {
			s.Latitude, s.Longitude = lat, lon
			if t, ok := parseBluelinkTime(getString(state, "vehicleLocation.time")); ok {
				s.LocationUpdatedAt = &t
			}
		}
	}

	if t, ok := parseBluelinkTime(getString(state, "vehicleStatus.time")); ok {
		s.LastUpdatedAt = &t
	}
	return s, nil
}

// ccs1TireWarning ORs the aggregate and per-corner low-pressure lamps. Returns nil
// when none are present (mirrors tireWarning for CCS2).
func ccs1TireWarning(state map[string]any) *bool {
	paths := []string{
		"vehicleStatus.tirePressureLamp.tirePressureLampAll",
		"vehicleStatus.tirePressureLamp.tirePressureLampFL",
		"vehicleStatus.tirePressureLamp.tirePressureLampFR",
		"vehicleStatus.tirePressureLamp.tirePressureLampRL",
		"vehicleStatus.tirePressureLamp.tirePressureLampRR",
	}
	warn, seen := false, false
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

// getBool returns a pointer to the bool at path, or nil when absent / not a bool.
func getBool(m map[string]any, path string) *bool {
	if b, ok := getPath(m, path).(bool); ok {
		return &b
	}
	return nil
}
