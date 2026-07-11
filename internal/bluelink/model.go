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

// InDistanceUnit returns a copy of the state with the odometer converted to unit
// ("km"/"mi") and OdometerUnit set to it. Unlike range — which the car already
// reports in its display unit and we publish unconverted — the odometer value is
// genuinely in its OdometerUnit (km for CCS2), so honouring DISTANCE_UNIT needs a
// real conversion. Returns the state unchanged when unit is empty, the odometer is
// absent, or it already matches unit (preserving the raw reported value); otherwise
// the converted value is rounded to 1 dp. The new value goes through a fresh pointer
// so the caller's state is never mutated.
func (s VehicleState) InDistanceUnit(unit string) VehicleState {
	if unit == "" || s.Odometer == nil || s.OdometerUnit == unit {
		return s
	}
	v := convertDistance(*s.Odometer, s.OdometerUnit, unit)
	v = math.Round(v*10) / 10
	s.Odometer = &v
	s.OdometerUnit = unit
	return s
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
