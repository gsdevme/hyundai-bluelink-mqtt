package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
)

type fakeFetcher struct {
	mu          sync.Mutex
	cached      int
	forced      int
	plugged     bool
	failCachedN int // fail the first N cached calls
	battery     float64
}

func (f *fakeFetcher) CachedStatus(context.Context, bluelink.Vehicle) (bluelink.VehicleState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cached++
	if f.cached <= f.failCachedN {
		return bluelink.VehicleState{}, errors.New("transient")
	}
	b, p := f.battery, f.plugged
	return bluelink.VehicleState{EVBatteryPercentage: &b, PluggedIn: &p}, nil
}

func (f *fakeFetcher) ForceStatus(context.Context, bluelink.Vehicle) (bluelink.VehicleState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forced++
	b := f.battery + 1
	return bluelink.VehicleState{EVBatteryPercentage: &b}, nil
}

func (f *fakeFetcher) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cached, f.forced
}

type fakePublisher struct {
	mu    sync.Mutex
	count int
	last  bluelink.VehicleState
}

func (p *fakePublisher) PublishState(_ context.Context, st bluelink.VehicleState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count++
	p.last = st
	return nil
}

func (p *fakePublisher) publishCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

type fakeHealth struct {
	mu               sync.Mutex
	success, failure int
}

func (h *fakeHealth) MarkSuccess() { h.mu.Lock(); h.success++; h.mu.Unlock() }
func (h *fakeHealth) MarkFailure() { h.mu.Lock(); h.failure++; h.mu.Unlock() }

func baseConfig() Config {
	return Config{
		Vehicle:       bluelink.Vehicle{ID: "v1", VIN: "VIN", CCS2ProtocolSupport: 1},
		PollInterval:  1000 * time.Hour, // effectively disable poll ticks in force tests
		MaxRetries:    3,
		ForceLocation: time.UTC,
		Now:           time.Now,
		After:         time.After,
	}
}

func TestImmediatePollPublishes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeFetcher{plugged: true, battery: 62}
		p := &fakePublisher{}
		h := &fakeHealth{}
		s := New(f, p, h, baseConfig())

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { s.Run(ctx); close(done) }()

		synctest.Wait()
		if p.publishCount() != 1 {
			t.Fatalf("publish count = %d, want 1 (immediate poll)", p.publishCount())
		}
		if h.success != 1 {
			t.Fatalf("health success = %d, want 1", h.success)
		}
		cancel()
		<-done
	})
}

func TestDailyForceRefreshFires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeFetcher{plugged: true, battery: 62}
		p := &fakePublisher{}
		h := &fakeHealth{}
		cfg := baseConfig()
		cfg.ForceEnabled = true
		cfg.ForceHour = 5 // bubble starts at 2000-01-01 00:00 UTC => fires at +5h
		s := New(f, p, h, cfg)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { s.Run(ctx); close(done) }()

		synctest.Wait() // initial poll
		time.Sleep(6 * time.Hour)
		synctest.Wait()

		_, forced := f.counts()
		if forced != 1 {
			t.Fatalf("force count = %d, want 1", forced)
		}
		cancel()
		<-done
	})
}

func TestForceSkippedWhenUnplugged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeFetcher{plugged: false, battery: 62}
		p := &fakePublisher{}
		h := &fakeHealth{}
		cfg := baseConfig()
		cfg.ForceEnabled = true
		cfg.ForceHour = 5
		cfg.ForceOnlyWhenPluggedIn = true
		s := New(f, p, h, cfg)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { s.Run(ctx); close(done) }()

		synctest.Wait()
		time.Sleep(6 * time.Hour)
		synctest.Wait()

		_, forced := f.counts()
		if forced != 0 {
			t.Fatalf("force count = %d, want 0 (unplugged, gated)", forced)
		}
		cancel()
		<-done
	})
}

func TestTransientRetrySucceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeFetcher{plugged: true, battery: 62, failCachedN: 2} // first 2 cached fail
		p := &fakePublisher{}
		h := &fakeHealth{}
		s := New(f, p, h, baseConfig())

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { s.Run(ctx); close(done) }()

		// The immediate poll fails twice, backing off 1s then 2s. Advance the
		// fake clock past the backoff so the retry succeeds.
		time.Sleep(10 * time.Second)
		synctest.Wait()

		if p.publishCount() != 1 {
			t.Fatalf("publish count = %d, want 1 after retries", p.publishCount())
		}
		if h.failure != 0 {
			t.Fatalf("health failure = %d, want 0 (retries absorbed the transient errors)", h.failure)
		}
		cancel()
		<-done
	})
}

func TestUntilNextForce(t *testing.T) {
	loc := time.UTC
	s := New(nil, nil, nil, Config{ForceHour: 5, ForceMinute: 0, ForceLocation: loc, Now: time.Now, After: time.After})
	// 03:00 -> 2h until 05:00 today.
	now := time.Date(2026, 7, 10, 3, 0, 0, 0, loc)
	if d := s.untilNextForce(now); d != 2*time.Hour {
		t.Errorf("from 03:00 = %s, want 2h", d)
	}
	// 06:00 -> 23h until 05:00 tomorrow.
	now = time.Date(2026, 7, 10, 6, 0, 0, 0, loc)
	if d := s.untilNextForce(now); d != 23*time.Hour {
		t.Errorf("from 06:00 = %s, want 23h", d)
	}
}
