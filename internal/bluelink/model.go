// Package bluelink is a read-only client for the Hyundai Bluelink EU API.
//
// It authenticates via the headless OAuth2/IDP flow, registers a device, lists
// vehicles, and fetches cached or forced CCS2 status plus park location, mapping
// the results into the clean [VehicleState] type the rest of the app consumes.
package bluelink

import (
	"math"
	"time"
)

// Vehicle is a car on the account, with the protocol flag that decides how its
// status is parsed.
type Vehicle struct {
	ID                  string
	VIN                 string
	Name                string // account nickname
	Model               string // e.g. "INSTER"
	RegDate             string
	Type                string // "EV", "PHEV", "GN", ...
	CCS2ProtocolSupport int    // ccuCCS2ProtocolSupport; != 0 means CCS2
}

// IsCCS2 reports whether the vehicle uses the CCS2 protocol.
func (v Vehicle) IsCCS2() bool { return v.CCS2ProtocolSupport != 0 }

// VehicleState is the normalised metrics snapshot published to MQTT. Pointer
// fields distinguish "unknown/absent" (rendered as null / HA "unknown") from a
// real zero value. Units are normalised on ingest (temperatures in °C).
type VehicleState struct {
	EVBatteryPercentage *float64 `json:"ev_battery_percentage,omitempty"`
	EVBatterySoH        *float64 `json:"ev_battery_soh,omitempty"`

	EVRange     *float64 `json:"ev_range,omitempty"`
	EVRangeUnit string   `json:"ev_range_unit,omitempty"`

	Charging           *bool `json:"charging,omitempty"`
	PluggedIn          *bool `json:"plugged_in,omitempty"`
	ChargePortDoorOpen *bool `json:"charge_port_door,omitempty"`

	ChargeLimitAC *float64 `json:"charge_limit_ac,omitempty"`
	ChargeLimitDC *float64 `json:"charge_limit_dc,omitempty"`

	ChargingPowerKW          *float64 `json:"charging_power,omitempty"`
	EstChargeDurationMin     *int     `json:"est_charge_time,omitempty"`
	EstFastChargeDurationMin *int     `json:"est_fast_charge_time,omitempty"`

	Battery12VPercentage *int `json:"battery_12v,omitempty"`

	Odometer     *float64 `json:"odometer,omitempty"`
	OdometerUnit string   `json:"odometer_unit,omitempty"`

	OutsideTemperatureC *float64 `json:"outside_temp,omitempty"`
	InsideTemperatureC  *float64 `json:"inside_temp,omitempty"`

	Locked              *bool `json:"locked,omitempty"`
	TirePressureWarning *bool `json:"tire_warning,omitempty"`

	Latitude          *float64   `json:"latitude,omitempty"`
	Longitude         *float64   `json:"longitude,omitempty"`
	LocationUpdatedAt *time.Time `json:"location_updated_at,omitempty"`

	LastUpdatedAt *time.Time `json:"last_updated,omitempty"`
}

// InDistanceUnit returns a copy of the state with the odometer and the EV range
// converted to unit ("km"/"mi"), each with its unit field set to match. Both values
// are genuinely in their own reported unit — the odometer in km for CCS2, the range
// in whatever the driver's display is set to — so honouring DISTANCE_UNIT needs a
// real conversion of each, not just a relabel. Returns the state unchanged when unit
// is empty; individual values are left alone (see convertDistanceField) when absent,
// already in unit, or of unknown source unit.
func (s VehicleState) InDistanceUnit(unit string) VehicleState {
	if unit == "" {
		return s
	}
	s.Odometer, s.OdometerUnit = convertDistanceField(s.Odometer, s.OdometerUnit, unit)
	s.EVRange, s.EVRangeUnit = convertDistanceField(s.EVRange, s.EVRangeUnit, unit)
	return s
}

// convertDistanceField converts an optional distance from unit `from` to unit `to`,
// returning the new value (rounded to 1 dp, through a fresh pointer so the caller's
// state is never mutated) and the unit it is now in.
//
// Absent values, already-matching units and — importantly — unknown source units are
// passed through untouched. Stamping `to` onto a value we could not convert is
// exactly how a kilometre reading ends up labelled "mi", which Home Assistant then
// "converts" to a number 1.6x too large.
func convertDistanceField(value *float64, from, to string) (*float64, string) {
	if value == nil || from == "" || from == to {
		return value, from
	}
	v := math.Round(convertDistance(*value, from, to)*10) / 10
	return &v, to
}

// Tokens is the persisted authentication state, round-tripped through a
// [TokenStore] so restarts avoid a fresh (rate-limited) login.
type Tokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	DeviceID     string    `json:"device_id"`
	ValidUntil   time.Time `json:"valid_until"`
}

// Valid reports whether the access token is present and not past (or near) expiry.
func (t Tokens) Valid(now time.Time) bool {
	if t.AccessToken == "" {
		return false
	}
	// Refresh a little before actual expiry to avoid racing the clock.
	return now.Before(t.ValidUntil.Add(-2 * time.Minute))
}
