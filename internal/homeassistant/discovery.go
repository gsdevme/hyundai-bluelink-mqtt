package homeassistant

import (
	"encoding/json"
	"fmt"
)

// Config identifies the topics and device metadata for a vehicle's discovery.
type Config struct {
	DiscoveryPrefix string // e.g. "homeassistant"
	TopicPrefix     string // e.g. "hyundai_bluelink"
	VIN             string
	Model           string // e.g. "INSTER"
	Name            string // device name (nickname or default)
	DistanceUnit    string // "km" | "mi"; Range + Odometer sensor label (defaults to "km")
}

// Message is a single MQTT publish (topic + payload); discovery messages are
// always retained at QoS 1.
type Message struct {
	Topic   string
	Payload []byte
}

// BaseTopic is the per-vehicle base topic used as the `~` abbreviation.
func (c Config) BaseTopic() string {
	return c.TopicPrefix + "/" + c.VIN
}

// StateTopic is the single retained JSON state topic.
func (c Config) StateTopic() string { return c.BaseTopic() + "/state" }

// AvailabilityTopic is the LWT/availability topic.
func (c Config) AvailabilityTopic() string { return c.BaseTopic() + "/availability" }

// TrackerStateTopic carries home/not_home/None for the device_tracker.
func (c Config) TrackerStateTopic() string { return c.BaseTopic() + "/tracker/state" }

// TrackerAttributesTopic carries the GPS attributes JSON.
func (c Config) TrackerAttributesTopic() string { return c.BaseTopic() + "/tracker/attributes" }

// BuildDiscovery returns the retained discovery config messages for every entity.
func BuildDiscovery(c Config) ([]Message, error) {
	device := map[string]any{
		"identifiers":  []string{c.VIN},
		"manufacturer": "Hyundai",
		"model":        modelOrDefault(c.Model),
		"name":         nameOrDefault(c.Name),
	}
	entities := Entities(c.DistanceUnit)
	msgs := make([]Message, 0, len(entities))
	for _, e := range entities {
		payload := buildEntityPayload(c, e, device)
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal discovery for %s: %w", e.Key, err)
		}
		topic := fmt.Sprintf("%s/%s/%s_%s/config", c.DiscoveryPrefix, e.Component, c.VIN, e.Key)
		msgs = append(msgs, Message{Topic: topic, Payload: body})
	}
	return msgs, nil
}

func buildEntityPayload(c Config, e Entity, device map[string]any) map[string]any {
	uniq := c.VIN + "_" + e.Key
	p := map[string]any{
		"~":                     c.BaseTopic(),
		"name":                  e.Name,
		"unique_id":             uniq,
		"object_id":             uniq,
		"availability_topic":    "~/availability",
		"payload_available":     "online",
		"payload_not_available": "offline",
		"device":                device,
	}
	if e.DeviceClass != "" {
		p["device_class"] = e.DeviceClass
	}
	if e.StateClass != "" {
		p["state_class"] = e.StateClass
	}
	if e.Unit != "" {
		p["unit_of_measurement"] = e.Unit
	}
	if e.Category != "" {
		p["entity_category"] = e.Category
	}

	switch e.Component {
	case DeviceTracker:
		p["state_topic"] = "~/tracker/state"
		p["json_attributes_topic"] = "~/tracker/attributes"
		p["source_type"] = "gps"
	case BinarySensor:
		p["state_topic"] = "~/state"
		p["payload_on"] = "ON"
		p["payload_off"] = "OFF"
		p["value_template"] = binarySensorTemplate(e.Key, e.InvertBool)
	default: // Sensor
		p["state_topic"] = "~/state"
		p["value_template"] = unknownGuardedTemplate(e.Key, "value_json."+e.Key)
	}
	return p
}

// unknownGuardedTemplate wraps expr in a Jinja conditional that renders the
// literal string `None` when value_json.<key> is absent or null.
//
// VehicleState omits unknown fields from the state JSON (pointer fields with
// `omitempty`), and HA's MQTT sensor and binary_sensor treat a rendered `None`
// as "reset to unknown". Without this guard a missing key renders as an
// undefined template (sensor) or as a falsy OFF/ON (binary sensor), which
// misreports an unknown value as a real one.
func unknownGuardedTemplate(key, expr string) string {
	return fmt.Sprintf("{{ %s if value_json.%s is defined and value_json.%s is not none else 'None' }}", expr, key, key)
}

// binarySensorTemplate renders value_json.<key> as ON/OFF, guarded by
// unknownGuardedTemplate so an absent or null key yields `None` (unknown).
//
// When invert is set (HA `lock` device class, where ON means unlocked) a truthy
// value renders OFF and a falsy one ON.
func binarySensorTemplate(key string, invert bool) string {
	on, off := "ON", "OFF"
	if invert {
		on, off = off, on
	}
	return unknownGuardedTemplate(key, fmt.Sprintf("('%s' if value_json.%s else '%s')", on, key, off))
}

func modelOrDefault(m string) string {
	if m == "" {
		return "Inster"
	}
	return m
}

func nameOrDefault(n string) string {
	if n == "" {
		return "Hyundai Inster"
	}
	return n
}
