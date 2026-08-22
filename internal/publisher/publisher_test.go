package publisher

import (
	"encoding/json"
	"testing"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/homeassistant"
)

func fp(v float64) *float64 { return &v }
func bp(v bool) *bool       { return &v }

func newService() (*Service, *RecordingPublisher) {
	rp := NewRecordingPublisher()
	cfg := homeassistant.Config{
		DiscoveryPrefix: "homeassistant",
		TopicPrefix:     "hyundai_bluelink",
		VIN:             "VIN123",
		Model:           "INSTER",
		Name:            "Inster",
	}
	return New(rp, cfg), rp
}

func TestPublishDiscoveryRetained(t *testing.T) {
	s, rp := newService()
	if err := s.PublishDiscovery(t.Context()); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	rec, ok := rp.Get("homeassistant/sensor/VIN123_ev_battery_percentage/config")
	if !ok {
		t.Fatal("battery discovery not published")
	}
	if !rec.Retain {
		t.Error("discovery must be retained")
	}
}

func TestPublishStateJSONAndTracker(t *testing.T) {
	s, rp := newService()
	st := bluelink.VehicleState{
		EVBatteryPercentage: fp(62),
		Charging:            bp(true),
		Latitude:            fp(51.5),
		Longitude:           fp(-0.12),
	}
	if err := s.PublishState(t.Context(), st); err != nil {
		t.Fatalf("state: %v", err)
	}

	rec, ok := rp.Get("hyundai_bluelink/VIN123/state")
	if !ok || !rec.Retain {
		t.Fatal("state not published retained")
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Payload, &got); err != nil {
		t.Fatalf("state payload: %v", err)
	}
	if got["ev_battery_percentage"] != 62.0 {
		t.Errorf("battery in state = %v", got["ev_battery_percentage"])
	}
	if got["charging"] != true {
		t.Errorf("charging in state = %v", got["charging"])
	}

	trState, ok := rp.Get("hyundai_bluelink/VIN123/tracker/state")
	if !ok || string(trState.Payload) != "not_home" {
		t.Errorf("tracker state = %q", trState.Payload)
	}
	trAttr, ok := rp.Get("hyundai_bluelink/VIN123/tracker/attributes")
	if !ok {
		t.Fatal("tracker attributes not published")
	}
	var attrs map[string]any
	_ = json.Unmarshal(trAttr.Payload, &attrs)
	if attrs["source_type"] != "gps" || attrs["latitude"] != 51.5 {
		t.Errorf("tracker attributes = %v", attrs)
	}
}

func TestPublishStateNoLocation(t *testing.T) {
	s, rp := newService()
	if err := s.PublishState(t.Context(), bluelink.VehicleState{EVBatteryPercentage: fp(50)}); err != nil {
		t.Fatalf("state: %v", err)
	}
	trState, ok := rp.Get("hyundai_bluelink/VIN123/tracker/state")
	if !ok || string(trState.Payload) != "None" {
		t.Errorf("tracker state = %q, want None", trState.Payload)
	}
}

func TestPublishStateOdometerDistanceUnit(t *testing.T) {
	stateOf := func(s *Service) map[string]any {
		rp := NewRecordingPublisher()
		s.pub = rp
		st := bluelink.VehicleState{
			Odometer: fp(14213), OdometerUnit: "km",
			EVRange: fp(259.99), EVRangeUnit: "km",
		}
		if err := s.PublishState(t.Context(), st); err != nil {
			t.Fatalf("state: %v", err)
		}
		rec, ok := rp.Get("hyundai_bluelink/VIN123/state")
		if !ok {
			t.Fatal("state not published")
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Payload, &got); err != nil {
			t.Fatalf("state payload: %v", err)
		}
		return got
	}

	check := func(t *testing.T, got map[string]any, wantOdo, wantRange float64, wantUnit string) {
		t.Helper()
		if v := got["odometer"]; v != wantOdo {
			t.Errorf("odometer = %v, want %v", v, wantOdo)
		}
		if v := got["odometer_unit"]; v != wantUnit {
			t.Errorf("odometer_unit = %v, want %q", v, wantUnit)
		}
		if v := got["ev_range"]; v != wantRange {
			t.Errorf("ev_range = %v, want %v", v, wantRange)
		}
		if v := got["ev_range_unit"]; v != wantUnit {
			t.Errorf("ev_range_unit = %v, want %q", v, wantUnit)
		}
	}

	// Default (km) publishes the raw km values untouched.
	s, _ := newService()
	check(t, stateOf(s), 14213.0, 259.99, "km")

	// DISTANCE_UNIT=mi converts both values (14213 km -> 8831.5 mi,
	// 259.99 km -> 161.6 mi, 1 dp) and relabels them.
	cfg := s.cfg
	cfg.DistanceUnit = "mi"
	check(t, stateOf(New(nil, cfg)), 8831.5, 161.6, "mi")
}

func TestPublishAvailability(t *testing.T) {
	s, rp := newService()
	_ = s.PublishAvailability(t.Context(), true)
	rec, _ := rp.Get("hyundai_bluelink/VIN123/availability")
	if string(rec.Payload) != "online" || !rec.Retain {
		t.Errorf("availability = %q retain=%v", rec.Payload, rec.Retain)
	}
	_ = s.PublishAvailability(t.Context(), false)
	rec, _ = rp.Get("hyundai_bluelink/VIN123/availability")
	if string(rec.Payload) != "offline" {
		t.Errorf("availability = %q, want offline", rec.Payload)
	}
}
