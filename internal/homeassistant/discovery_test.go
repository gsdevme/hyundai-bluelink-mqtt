package homeassistant

import (
	"encoding/json"
	"testing"
)

func testConfig() Config {
	return Config{
		DiscoveryPrefix: "homeassistant",
		TopicPrefix:     "hyundai_bluelink",
		VIN:             "VIN123",
		Model:           "INSTER",
		Name:            "Inster",
	}
}

func decodeByKey(t *testing.T, msgs []Message) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, m := range msgs {
		var p map[string]any
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			t.Fatalf("bad payload for %s: %v", m.Topic, err)
		}
		out[m.Topic] = p
	}
	return out
}

func TestBuildDiscoveryTopicsAndCount(t *testing.T) {
	msgs, err := BuildDiscovery(testConfig())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(msgs) != len(Entities("")) {
		t.Fatalf("got %d messages, want %d", len(msgs), len(Entities("")))
	}
	byTopic := decodeByKey(t, msgs)

	battery, ok := byTopic["homeassistant/sensor/VIN123_ev_battery_percentage/config"]
	if !ok {
		t.Fatal("missing battery discovery topic")
	}
	if battery["~"] != "hyundai_bluelink/VIN123" {
		t.Errorf("~ = %v", battery["~"])
	}
	if battery["state_topic"] != "~/state" {
		t.Errorf("state_topic = %v", battery["state_topic"])
	}
	if battery["availability_topic"] != "~/availability" {
		t.Errorf("availability_topic = %v", battery["availability_topic"])
	}
	if battery["value_template"] != "{{ value_json.ev_battery_percentage }}" {
		t.Errorf("value_template = %v", battery["value_template"])
	}
	if battery["device_class"] != "battery" || battery["state_class"] != "measurement" {
		t.Errorf("battery classes = %v / %v", battery["device_class"], battery["state_class"])
	}
	if battery["unique_id"] != "VIN123_ev_battery_percentage" {
		t.Errorf("unique_id = %v", battery["unique_id"])
	}
	dev, _ := battery["device"].(map[string]any)
	if dev == nil || dev["manufacturer"] != "Hyundai" || dev["model"] != "INSTER" {
		t.Errorf("device block = %v", battery["device"])
	}
	ids, _ := dev["identifiers"].([]any)
	if len(ids) != 1 || ids[0] != "VIN123" {
		t.Errorf("identifiers = %v", dev["identifiers"])
	}
}

func TestOdometerStateClassTotal(t *testing.T) {
	byTopic := decodeByKey(t, mustBuild(t))
	odo := byTopic["homeassistant/sensor/VIN123_odometer/config"]
	if odo["state_class"] != "total" {
		t.Errorf("odometer state_class = %v, want total (tolerates resets)", odo["state_class"])
	}
}

func TestDiagnosticCategory(t *testing.T) {
	byTopic := decodeByKey(t, mustBuild(t))
	for _, key := range []string{"battery_12v", "charge_limit_ac", "charge_limit_dc", "ev_battery_soh", "last_updated"} {
		p := byTopic["homeassistant/sensor/VIN123_"+key+"/config"]
		if p["entity_category"] != "diagnostic" {
			t.Errorf("%s entity_category = %v, want diagnostic", key, p["entity_category"])
		}
	}
	port := byTopic["homeassistant/binary_sensor/VIN123_charge_port_door/config"]
	if port["entity_category"] != "diagnostic" {
		t.Errorf("charge_port_door category = %v", port["entity_category"])
	}
}

func TestBinarySensorTemplates(t *testing.T) {
	byTopic := decodeByKey(t, mustBuild(t))
	charging := byTopic["homeassistant/binary_sensor/VIN123_charging/config"]
	if charging["value_template"] != "{{ 'ON' if value_json.charging else 'OFF' }}" {
		t.Errorf("charging template = %v", charging["value_template"])
	}
	if charging["device_class"] != "battery_charging" {
		t.Errorf("charging device_class = %v", charging["device_class"])
	}
	// lock class inverts.
	locked := byTopic["homeassistant/binary_sensor/VIN123_locked/config"]
	if locked["value_template"] != "{{ 'OFF' if value_json.locked else 'ON' }}" {
		t.Errorf("locked template = %v", locked["value_template"])
	}
}

func TestDeviceTracker(t *testing.T) {
	byTopic := decodeByKey(t, mustBuild(t))
	tr := byTopic["homeassistant/device_tracker/VIN123_location/config"]
	if tr["state_topic"] != "~/tracker/state" {
		t.Errorf("tracker state_topic = %v", tr["state_topic"])
	}
	if tr["json_attributes_topic"] != "~/tracker/attributes" {
		t.Errorf("tracker attributes topic = %v", tr["json_attributes_topic"])
	}
	if tr["source_type"] != "gps" {
		t.Errorf("tracker source_type = %v", tr["source_type"])
	}
}

func TestDistanceUnitFromConfig(t *testing.T) {
	// Default (empty) config leaves both Range and Odometer labelled km.
	byTopic := decodeByKey(t, mustBuild(t))
	if u := byTopic["homeassistant/sensor/VIN123_ev_range/config"]["unit_of_measurement"]; u != "km" {
		t.Errorf("default ev_range unit = %v, want km", u)
	}
	if u := byTopic["homeassistant/sensor/VIN123_odometer/config"]["unit_of_measurement"]; u != "km" {
		t.Errorf("default odometer unit = %v, want km", u)
	}

	// DistanceUnit=mi labels both Range and Odometer mi. Both values are converted
	// to match at publish time — see the publisher and bluelink.InDistanceUnit tests.
	cfg := testConfig()
	cfg.DistanceUnit = "mi"
	msgs, err := BuildDiscovery(cfg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	byTopic = decodeByKey(t, msgs)
	if u := byTopic["homeassistant/sensor/VIN123_ev_range/config"]["unit_of_measurement"]; u != "mi" {
		t.Errorf("ev_range unit = %v, want mi", u)
	}
	if u := byTopic["homeassistant/sensor/VIN123_odometer/config"]["unit_of_measurement"]; u != "mi" {
		t.Errorf("odometer unit = %v, want mi", u)
	}
}

func mustBuild(t *testing.T) []Message {
	t.Helper()
	msgs, err := BuildDiscovery(testConfig())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return msgs
}
