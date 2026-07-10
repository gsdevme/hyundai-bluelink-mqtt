// Package server exposes the daemon's HTTP surface: a human-friendly status page
// on / plus liveness (/healthz) and readiness (/readyz) probes. Liveness is
// always OK while the process runs; readiness flips ready after the first
// successful publish and not-ready after a configurable number of consecutive
// poll failures. See docs/specs/06-lifecycle-health.md.
package server

import (
	"net/http"
	"sync"
	"time"
)

// Config configures the status server. It is deliberately decoupled from
// internal/config and internal/bluelink so serve.go maps domain values in and
// there is no import coupling here.
type Config struct {
	ReadyFailureThreshold int
	PollInterval          time.Duration
	ForceEnabled          bool
	ForceHour             int
	ForceMinute           int
	ForceLocation         *time.Location
}

// Server tracks readiness based on poll outcomes and holds a concurrency-safe
// snapshot of operational status for the / page.
type Server struct {
	mu                  sync.Mutex
	ready               bool
	consecutiveFailures int
	threshold           int

	// Vehicle status, set after selection (see SetVehicle). Until then the /
	// page renders these as "initialising".
	vehicleSet   bool
	vehicleModel string
	vehicleName  string
	vehicleVIN   string
	vehicleCCS2  bool

	cfg       Config
	startedAt time.Time
}

// New returns a Server that flips not-ready after cfg.ReadyFailureThreshold
// consecutive failures. A threshold below 1 is treated as 1.
func New(cfg Config) *Server {
	if cfg.ReadyFailureThreshold < 1 {
		cfg.ReadyFailureThreshold = 1
	}
	return &Server{threshold: cfg.ReadyFailureThreshold, cfg: cfg, startedAt: time.Now()}
}

// MarkSuccess records a successful publish: ready becomes true and the failure
// counter resets.
func (s *Server) MarkSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
	s.consecutiveFailures = 0
}

// MarkFailure records a poll failure; readiness flips off once the threshold of
// consecutive failures is reached.
func (s *Server) MarkFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures++
	if s.consecutiveFailures >= s.threshold {
		s.ready = false
	}
}

// Ready reports the current readiness state.
func (s *Server) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready
}

// SetVehicle records the selected vehicle for the status page. It is called
// after vehicle selection; the server starts listening before selection, so the
// / page renders vehicle fields as "initialising" until this runs.
func (s *Server) SetVehicle(model, name, vin string, ccs2 bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vehicleSet = true
	s.vehicleModel = model
	s.vehicleName = name
	s.vehicleVIN = vin
	s.vehicleCCS2 = ccs2
}

// Handler returns the mux serving /, /healthz and /readyz. See routes.go for the
// route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, s)
	return mux
}

func (s *Server) handleLivez(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if s.Ready() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("not ready"))
}
