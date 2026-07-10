// Package scheduler runs the cached poll loop and the daily force-refresh
// schedule, feeding parsed VehicleState to the publisher and reporting outcomes
// to health. See docs/specs/04-polling-scheduling.md.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
)

// StatusFetcher fetches vehicle status (cached or forced).
type StatusFetcher interface {
	CachedStatus(ctx context.Context, v bluelink.Vehicle) (bluelink.VehicleState, error)
	ForceStatus(ctx context.Context, v bluelink.Vehicle) (bluelink.VehicleState, error)
}

// StatePublisher publishes a vehicle state.
type StatePublisher interface {
	PublishState(ctx context.Context, st bluelink.VehicleState) error
}

// HealthReporter records poll outcomes for readiness.
type HealthReporter interface {
	MarkSuccess()
	MarkFailure()
}

// Config configures the scheduler.
type Config struct {
	Vehicle      bluelink.Vehicle
	PollInterval time.Duration
	MaxRetries   int

	ForceEnabled           bool
	ForceHour              int
	ForceMinute            int
	ForceLocation          *time.Location
	ForceOnlyWhenPluggedIn bool

	Logger *slog.Logger
	// Now and After are injectable for deterministic tests (testing/synctest).
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
}

// Scheduler owns the poll and force-refresh loops.
type Scheduler struct {
	fetcher   StatusFetcher
	publisher StatePublisher
	health    HealthReporter
	cfg       Config
	logger    *slog.Logger
	now       func() time.Time
	after     func(time.Duration) <-chan time.Time

	// apiMu serialises API calls so a force refresh and a cached poll never
	// overlap on the shared HTTP client / token refresh.
	apiMu sync.Mutex
}

// New builds a Scheduler.
func New(f StatusFetcher, p StatePublisher, h HealthReporter, cfg Config) *Scheduler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.After == nil {
		cfg.After = time.After
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	return &Scheduler{
		fetcher: f, publisher: p, health: h, cfg: cfg,
		logger: cfg.Logger, now: cfg.Now, after: cfg.After,
	}
}

// Run starts both loops and blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); s.pollLoop(ctx) }()
	if s.cfg.ForceEnabled {
		wg.Add(1)
		go func() { defer wg.Done(); s.forceLoop(ctx) }()
	}
	wg.Wait()
}

// PollNow runs a single cached poll cycle (fetch + publish + health). Exposed
// for the acceptance suite; the timed loop uses the same underlying logic.
func (s *Scheduler) PollNow(ctx context.Context) { s.poll(ctx) }

// ForceNow runs a single force-refresh cycle, including the plugged-in gate.
// Exposed for the acceptance suite.
func (s *Scheduler) ForceNow(ctx context.Context) { s.forceRefresh(ctx) }

func (s *Scheduler) pollLoop(ctx context.Context) {
	s.poll(ctx) // immediate first poll
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll performs one cached read + publish, updating health.
func (s *Scheduler) poll(ctx context.Context) {
	s.apiMu.Lock()
	defer s.apiMu.Unlock()

	st, err := s.fetchWithRetry(ctx, false)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.logger.WarnContext(ctx, "cached poll failed", "err", err)
		s.health.MarkFailure()
		return
	}
	if err := s.publisher.PublishState(ctx, st); err != nil {
		s.logger.WarnContext(ctx, "publish failed", "err", err)
		s.health.MarkFailure()
		return
	}
	s.logger.InfoContext(ctx, "published cached state", "battery", derefFloat(st.EVBatteryPercentage))
	s.health.MarkSuccess()
}

func (s *Scheduler) forceLoop(ctx context.Context) {
	for {
		wait := s.untilNextForce(s.now())
		select {
		case <-ctx.Done():
			return
		case <-s.after(wait):
			s.forceRefresh(ctx)
		}
	}
}

// forceRefresh optionally checks plugged-in via a cached read, then performs a
// forced (car-waking) refresh and publishes it.
func (s *Scheduler) forceRefresh(ctx context.Context) {
	s.apiMu.Lock()
	defer s.apiMu.Unlock()

	if s.cfg.ForceOnlyWhenPluggedIn {
		cached, err := s.fetcher.CachedStatus(ctx, s.cfg.Vehicle)
		if err != nil {
			s.logger.WarnContext(ctx, "force-refresh plugged-in pre-check failed", "err", err)
			return
		}
		if cached.PluggedIn == nil || !*cached.PluggedIn {
			s.logger.InfoContext(ctx, "skipping daily force refresh: vehicle not plugged in")
			return
		}
	}

	st, err := s.fetchWithRetry(ctx, true)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.logger.WarnContext(ctx, "force refresh failed", "err", err)
		s.health.MarkFailure()
		return
	}
	if err := s.publisher.PublishState(ctx, st); err != nil {
		s.logger.WarnContext(ctx, "publish (force) failed", "err", err)
		s.health.MarkFailure()
		return
	}
	s.logger.InfoContext(ctx, "published forced state", "battery", derefFloat(st.EVBatteryPercentage))
	s.health.MarkSuccess()
}

// fetchWithRetry fetches status, retrying transient errors with exponential
// backoff up to MaxRetries.
func (s *Scheduler) fetchWithRetry(ctx context.Context, force bool) (bluelink.VehicleState, error) {
	var lastErr error
	backoff := time.Second
	for attempt := 0; attempt <= s.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return bluelink.VehicleState{}, ctx.Err()
			case <-s.after(backoff):
			}
			backoff *= 2
		}
		var st bluelink.VehicleState
		var err error
		if force {
			st, err = s.fetcher.ForceStatus(ctx, s.cfg.Vehicle)
		} else {
			st, err = s.fetcher.CachedStatus(ctx, s.cfg.Vehicle)
		}
		if err == nil {
			return st, nil
		}
		if ctx.Err() != nil {
			return bluelink.VehicleState{}, ctx.Err()
		}
		lastErr = err
		s.logger.DebugContext(ctx, "status fetch attempt failed", "attempt", attempt, "err", err)
	}
	return bluelink.VehicleState{}, lastErr
}

// untilNextForce returns the duration until the next scheduled force refresh,
// recomputed against the location each time so DST transitions are handled.
func (s *Scheduler) untilNextForce(now time.Time) time.Duration {
	loc := s.cfg.ForceLocation
	if loc == nil {
		loc = time.UTC
	}
	n := now.In(loc)
	next := time.Date(n.Year(), n.Month(), n.Day(), s.cfg.ForceHour, s.cfg.ForceMinute, 0, 0, loc)
	if !next.After(n) {
		next = time.Date(n.Year(), n.Month(), n.Day()+1, s.cfg.ForceHour, s.cfg.ForceMinute, 0, 0, loc)
	}
	return next.Sub(now)
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}
