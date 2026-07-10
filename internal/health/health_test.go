package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadinessLifecycle(t *testing.T) {
	h := New(3)
	if h.Ready() {
		t.Fatal("should start not ready")
	}
	h.MarkSuccess()
	if !h.Ready() {
		t.Fatal("should be ready after first success")
	}
	h.MarkFailure()
	h.MarkFailure()
	if !h.Ready() {
		t.Fatal("should stay ready below threshold")
	}
	h.MarkFailure() // 3rd consecutive
	if h.Ready() {
		t.Fatal("should be not ready at threshold")
	}
	h.MarkSuccess()
	if !h.Ready() {
		t.Fatal("should recover on success")
	}
}

func TestHandlers(t *testing.T) {
	h := New(1)
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	// Liveness always OK.
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %v / %v", resp, err)
	}
	// Readiness 503 before first success.
	resp, _ = http.Get(srv.URL + "/readyz")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz before success = %d, want 503", resp.StatusCode)
	}
	h.MarkSuccess()
	resp, _ = http.Get(srv.URL + "/readyz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz after success = %d, want 200", resp.StatusCode)
	}
}
