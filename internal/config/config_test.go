package config

import (
	"strings"
	"testing"
)

// setEnv sets the minimum required vars plus any overrides for a test.
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	base := map[string]string{
		"BLUELINK_USERNAME": "user@example.com",
		"BLUELINK_PASSWORD": "secret",
		"MQTT_BROKER_URL":   "mqtt://localhost:1883",
	}
	for k, v := range base {
		if _, ok := kv[k]; !ok {
			t.Setenv(k, v)
		}
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, nil)
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.PollInterval.Minutes() != 30 {
		t.Errorf("poll interval = %s, want 30m", c.PollInterval)
	}
	if !c.ForceRefreshEnabled || c.ForceRefreshHour != 5 {
		t.Errorf("force refresh = %v @ %d", c.ForceRefreshEnabled, c.ForceRefreshHour)
	}
	if c.TokenStore != "memory" {
		t.Errorf("token store = %s", c.TokenStore)
	}
}

func TestMissingRequired(t *testing.T) {
	t.Setenv("BLUELINK_USERNAME", "")
	t.Setenv("BLUELINK_PASSWORD", "")
	t.Setenv("MQTT_BROKER_URL", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required vars")
	}
	for _, want := range []string{"BLUELINK_USERNAME", "BLUELINK_PASSWORD", "MQTT_BROKER_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %v", want, err)
		}
	}
}

func TestPollFloor(t *testing.T) {
	setEnv(t, map[string]string{"POLL_INTERVAL": "1m"})
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "floor") {
		t.Fatalf("expected poll floor error, got %v", err)
	}
}

func TestBadForceRefresh(t *testing.T) {
	setEnv(t, map[string]string{"FORCE_REFRESH_AT": "25:99"})
	if _, err := Load(); err == nil {
		t.Fatal("expected error for bad FORCE_REFRESH_AT")
	}
	setEnv(t, map[string]string{"FORCE_REFRESH_TZ": "Mars/Phobos"})
	if _, err := Load(); err == nil {
		t.Fatal("expected error for bad FORCE_REFRESH_TZ")
	}
}

func TestKubeRequiresSecret(t *testing.T) {
	setEnv(t, map[string]string{"TOKEN_STORE": "kube"})
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TOKEN_SECRET_NAME") {
		t.Fatalf("expected TOKEN_SECRET_NAME error, got %v", err)
	}
}

func TestForceRefreshTimezone(t *testing.T) {
	setEnv(t, map[string]string{"FORCE_REFRESH_AT": "06:30", "FORCE_REFRESH_TZ": "Europe/London"})
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.ForceRefreshHour != 6 || c.ForceRefreshMinute != 30 {
		t.Errorf("time = %02d:%02d", c.ForceRefreshHour, c.ForceRefreshMinute)
	}
	if c.ForceRefreshLocation.String() != "Europe/London" {
		t.Errorf("tz = %s", c.ForceRefreshLocation)
	}
}

func TestRedaction(t *testing.T) {
	setEnv(t, map[string]string{
		"BLUELINK_VIN":      "SECRETVIN",
		"BLUELINK_PASSWORD": "pw-should-not-appear",
		"MQTT_PASSWORD":     "hunter2",
	})
	c, _ := Load()
	s := c.String()
	for _, leak := range []string{"SECRETVIN", "hunter2", "pw-should-not-appear"} {
		if strings.Contains(s, leak) {
			t.Errorf("String() leaked %q: %s", leak, s)
		}
	}
}
