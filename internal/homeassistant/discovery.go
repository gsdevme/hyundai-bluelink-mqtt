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
	DistanceUnit    string // "km" | "mi"; Range sensor label (defaults to "km")
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
		if e.InvertBool {
			// HA `lock`: ON = unlocked, so emit ON when the value is falsy.
			p["value_template"] = fmt.Sprintf("{{ 'OFF' if value_json.%s else 'ON' }}", e.Key)
		} else {
			p["value_template"] = fmt.Sprintf("{{ 'ON' if value_json.%s else 'OFF' }}", e.Key)
		}
	default: // Sensor
		p["state_topic"] = "~/state"
		p["value_template"] = fmt.Sprintf("{{ value_json.%s }}", e.Key)
	}
	return p
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
