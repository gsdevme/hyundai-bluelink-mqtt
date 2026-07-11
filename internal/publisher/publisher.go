// Package publisher turns a VehicleState into retained MQTT messages: the Home
// Assistant discovery configs (once, on startup), the single JSON state document
// (per poll), the device-tracker state/attributes, and availability.
package publisher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/homeassistant"
)

// Publisher is the MQTT transport. Discovery/state/availability are always
// published retained; the transport uses QoS 1. Implementations: the autopaho
// client (prod) and a recording fake (tests).
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte, retain bool) error
}

// Service publishes HA discovery and vehicle state for one vehicle.
type Service struct {
	pub Publisher
	cfg homeassistant.Config
}

// New builds a Service from a Publisher and discovery config.
func New(pub Publisher, cfg homeassistant.Config) *Service {
	return &Service{pub: pub, cfg: cfg}
}

// AvailabilityTopic exposes the topic used for LWT and explicit availability.
func (s *Service) AvailabilityTopic() string { return s.cfg.AvailabilityTopic() }

// PublishDiscovery publishes every entity's retained discovery config.
func (s *Service) PublishDiscovery(ctx context.Context) error {
	msgs, err := homeassistant.BuildDiscovery(s.cfg)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if err := s.pub.Publish(ctx, m.Topic, m.Payload, true); err != nil {
			return fmt.Errorf("publish discovery %s: %w", m.Topic, err)
		}
	}
	return nil
}

// PublishAvailability publishes online/offline (retained) to the availability
// topic.
func (s *Service) PublishAvailability(ctx context.Context, online bool) error {
	payload := "offline"
	if online {
		payload = "online"
	}
	return s.pub.Publish(ctx, s.cfg.AvailabilityTopic(), []byte(payload), true)
}

// PublishState publishes the retained JSON state document and the device-tracker
// topics for the given vehicle state.
func (s *Service) PublishState(ctx context.Context, st bluelink.VehicleState) error {
	// Convert the odometer to the configured display unit before publishing so the
	// value matches the odometer entity's HA label (Range needs no conversion — the
	// car already reports it in its display unit).
	st = st.InDistanceUnit(s.cfg.DistanceUnit)
	body, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := s.pub.Publish(ctx, s.cfg.StateTopic(), body, true); err != nil {
		return fmt.Errorf("publish state: %w", err)
	}
	return s.publishTracker(ctx, st)
}

// publishTracker publishes device-tracker state + GPS attributes. The state is a
// neutral "not_home" (or "None" when unknown); HA maps the car from the
// coordinates in the attributes.
func (s *Service) publishTracker(ctx context.Context, st bluelink.VehicleState) error {
	if st.Latitude == nil || st.Longitude == nil {
		return s.pub.Publish(ctx, s.cfg.TrackerStateTopic(), []byte("None"), true)
	}
	attrs := map[string]any{
		"latitude":     *st.Latitude,
		"longitude":    *st.Longitude,
		"gps_accuracy": 1,
		"source_type":  "gps",
	}
	body, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("marshal tracker attributes: %w", err)
	}
	if err := s.pub.Publish(ctx, s.cfg.TrackerAttributesTopic(), body, true); err != nil {
		return fmt.Errorf("publish tracker attributes: %w", err)
	}
	return s.pub.Publish(ctx, s.cfg.TrackerStateTopic(), []byte("not_home"), true)
}
