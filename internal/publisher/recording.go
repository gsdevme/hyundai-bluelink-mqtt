package publisher

import (
	"context"
	"sync"
)

// Record is a captured MQTT publish.
type Record struct {
	Payload []byte
	Retain  bool
}

// RecordingPublisher is a Publisher that stores the last payload per topic. It
// lets tests and the godog suite assert on MQTT output without a broker.
type RecordingPublisher struct {
	mu      sync.Mutex
	records map[string]Record
	order   []string
}

// NewRecordingPublisher returns an empty RecordingPublisher.
func NewRecordingPublisher() *RecordingPublisher {
	return &RecordingPublisher{records: map[string]Record{}}
}

// Publish records the message (last write wins per topic).
func (r *RecordingPublisher) Publish(_ context.Context, topic string, payload []byte, retain bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, seen := r.records[topic]; !seen {
		r.order = append(r.order, topic)
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	r.records[topic] = Record{Payload: cp, Retain: retain}
	return nil
}

// Get returns the last record for a topic.
func (r *RecordingPublisher) Get(topic string) (Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[topic]
	return rec, ok
}

// Topics returns all published topics in first-seen order.
func (r *RecordingPublisher) Topics() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}
