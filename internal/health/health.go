// Package health exposes liveness (/healthz) and readiness (/readyz) HTTP
// endpoints. Liveness is always OK while the process runs; readiness flips ready
// after the first successful publish and not-ready after a configurable number
// of consecutive poll failures. See docs/specs/06-lifecycle-health.md.
package health

import (
	"net/http"
	"sync"
)

// Health tracks readiness based on poll outcomes.
type Health struct {
	mu                  sync.Mutex
	ready               bool
	consecutiveFailures int
	threshold           int
}

// New returns a Health that flips not-ready after `threshold` consecutive
// failures. A threshold below 1 is treated as 1.
func New(threshold int) *Health {
	if threshold < 1 {
		threshold = 1
	}
	return &Health{threshold: threshold}
}

// MarkSuccess records a successful publish: ready becomes true and the failure
// counter resets.
func (h *Health) MarkSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = true
	h.consecutiveFailures = 0
}

// MarkFailure records a poll failure; readiness flips off once the threshold of
// consecutive failures is reached.
func (h *Health) MarkFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.consecutiveFailures++
	if h.consecutiveFailures >= h.threshold {
		h.ready = false
	}
}

// Ready reports the current readiness state.
func (h *Health) Ready() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ready
}

// Handler returns the mux serving /healthz and /readyz.
func (h *Health) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if h.Ready() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
	})
	return mux
}
