package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadinessLifecycle(t *testing.T) {
	s := New(Config{ReadyFailureThreshold: 3})
	if s.Ready() {
		t.Fatal("should start not ready")
	}
	s.MarkSuccess()
	if !s.Ready() {
		t.Fatal("should be ready after first success")
	}
	s.MarkFailure()
	s.MarkFailure()
	if !s.Ready() {
		t.Fatal("should stay ready below threshold")
	}
	s.MarkFailure() // 3rd consecutive
	if s.Ready() {
		t.Fatal("should be not ready at threshold")
	}
	s.MarkSuccess()
	if !s.Ready() {
		t.Fatal("should recover on success")
	}
}

func TestProbes(t *testing.T) {
	s := New(Config{ReadyFailureThreshold: 1})
	srv := httptest.NewServer(s.Handler())
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
	s.MarkSuccess()
	resp, _ = http.Get(srv.URL + "/readyz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz after success = %d, want 200", resp.StatusCode)
	}
}

func TestRootStatusPage(t *testing.T) {
	s := New(Config{ReadyFailureThreshold: 1})
	s.SetVehicle("INSTER", "Inster", "REDACTEDVIN000001", true)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	page := string(body)
	if !strings.Contains(page, "not ready") {
		t.Fatalf("page missing readiness status:\n%s", page)
	}
	if !strings.Contains(page, "0001") {
		t.Fatalf("page missing masked VIN suffix:\n%s", page)
	}
	if strings.Contains(page, "REDACTEDVIN000001") {
		t.Fatalf("page leaked the full VIN:\n%s", page)
	}
}

func TestUnknownPath404(t *testing.T) {
	s := New(Config{ReadyFailureThreshold: 1})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatalf("GET /nope error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /nope = %d, want 404", resp.StatusCode)
	}
}
