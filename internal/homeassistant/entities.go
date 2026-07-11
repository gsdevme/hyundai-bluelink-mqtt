// Package homeassistant builds Home Assistant MQTT autodiscovery payloads for a
// vehicle. See docs/specs/03-mqtt-ha-discovery.md and the
// home-assistant-mqtt-discovery skill, which are the source of truth for these
// rules.
package homeassistant

// Component types.
const (
	Sensor        = "sensor"
	BinarySensor  = "binary_sensor"
	DeviceTracker = "device_tracker"
)

// Entity describes a single HA entity derived from the shared JSON state topic.
// Key is both the discovery object-id suffix and the JSON field the entity reads
// (value_json.<Key>), so entity keys must match VehicleState JSON tags.
type Entity struct {
	Component   string
	Key         string
	Name        string
	DeviceClass string
	StateClass  string
	Unit        string
	Category    string // "diagnostic" or empty
	// InvertBool, for binary_sensors, emits ON when the JSON value is falsy
	// (used for the HA `lock` class where ON = unlocked).
	InvertBool bool
}

// Entities returns the full catalogue published for a vehicle. distanceUnit sets
// the Range sensor's HA label ("km" or "mi"); it defaults to "km" when empty. The
// value is a label only — the car reports range in its own display unit and we
// publish that number unconverted. (Odometer is always km as reported by the API.)
func Entities(distanceUnit string) []Entity {
	if distanceUnit == "" {
		distanceUnit = "km"
	}
	return []Entity{
		// Sensors.
		{Component: Sensor, Key: "ev_battery_percentage", Name: "Battery", DeviceClass: "battery", StateClass: "measurement", Unit: "%"},
		{Component: Sensor, Key: "ev_range", Name: "Range", DeviceClass: "distance", StateClass: "measurement", Unit: distanceUnit},
		{Component: Sensor, Key: "charging_power", Name: "Charging power", DeviceClass: "power", StateClass: "measurement", Unit: "kW"},
		{Component: Sensor, Key: "battery_12v", Name: "12V battery", DeviceClass: "battery", StateClass: "measurement", Unit: "%", Category: "diagnostic"},
		{Component: Sensor, Key: "odometer", Name: "Odometer", DeviceClass: "distance", StateClass: "total", Unit: "km"},
		{Component: Sensor, Key: "charge_limit_ac", Name: "AC charge limit", StateClass: "measurement", Unit: "%", Category: "diagnostic"},
		{Component: Sensor, Key: "charge_limit_dc", Name: "DC charge limit", StateClass: "measurement", Unit: "%", Category: "diagnostic"},
		{Component: Sensor, Key: "est_charge_time", Name: "Charge time remaining", DeviceClass: "duration", StateClass: "measurement", Unit: "min"},
		{Component: Sensor, Key: "outside_temp", Name: "Outside temperature", DeviceClass: "temperature", StateClass: "measurement", Unit: "°C"},
		{Component: Sensor, Key: "ev_battery_soh", Name: "Battery health", StateClass: "measurement", Unit: "%", Category: "diagnostic"},
		{Component: Sensor, Key: "last_updated", Name: "Last updated", DeviceClass: "timestamp", Category: "diagnostic"},

		// Binary sensors.
		{Component: BinarySensor, Key: "charging", Name: "Charging", DeviceClass: "battery_charging"},
		{Component: BinarySensor, Key: "plugged_in", Name: "Plugged in", DeviceClass: "plug"},
		{Component: BinarySensor, Key: "locked", Name: "Doors", DeviceClass: "lock", InvertBool: true},
		{Component: BinarySensor, Key: "tire_warning", Name: "Tire pressure", DeviceClass: "problem"},
		{Component: BinarySensor, Key: "charge_port_door", Name: "Charge port", DeviceClass: "door", Category: "diagnostic"},

		// Location.
		{Component: DeviceTracker, Key: "location", Name: "Location"},
	}
}
