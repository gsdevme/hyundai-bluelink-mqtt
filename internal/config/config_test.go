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
	if c.PollInterval.Minutes() != 45 {
		t.Errorf("poll interval = %s, want 45m", c.PollInterval)
	}
	if !c.ForceRefreshEnabled || c.ForceRefreshHour != 5 {
		t.Errorf("force refresh = %v @ %d", c.ForceRefreshEnabled, c.ForceRefreshHour)
	}
	if c.TokenStore != "memory" {
		t.Errorf("token store = %s", c.TokenStore)
	}
	if c.DistanceUnit != "km" {
		t.Errorf("distance unit = %s, want km", c.DistanceUnit)
	}
}

func TestDistanceUnit(t *testing.T) {
	setEnv(t, map[string]string{"DISTANCE_UNIT": "MI"})
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.DistanceUnit != "mi" {
		t.Errorf("distance unit = %q, want mi (lowercased)", c.DistanceUnit)
	}
}

func TestBadDistanceUnit(t *testing.T) {
	setEnv(t, map[string]string{"DISTANCE_UNIT": "miles"})
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DISTANCE_UNIT") {
		t.Fatalf("expected DISTANCE_UNIT validation error, got %v", err)
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

func TestModeLiveDefault(t *testing.T) {
	setEnv(t, nil) // MODE unset
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Mode != "live" {
		t.Errorf("mode = %q, want live", c.Mode)
	}
	if c.BluelinkBaseURL != "" || c.BluelinkLoginURL != "" {
		t.Errorf("live hosts should be empty, got base=%q login=%q", c.BluelinkBaseURL, c.BluelinkLoginURL)
	}
}

func TestModeMockResolvesDefaultURL(t *testing.T) {
	// Empty creds must be accepted in mock mode.
	setEnv(t, map[string]string{"MODE": "mock", "BLUELINK_USERNAME": "", "BLUELINK_PASSWORD": ""})
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.BluelinkBaseURL != defaultMockURL || c.BluelinkLoginURL != defaultMockURL {
		t.Errorf("mock hosts = base=%q login=%q, want %q", c.BluelinkBaseURL, c.BluelinkLoginURL, defaultMockURL)
	}
	if c.BluelinkUsername == "" || c.BluelinkPassword == "" {
		t.Errorf("mock should supply dummy creds, got user=%q pass=%q", c.BluelinkUsername, c.BluelinkPassword)
	}
}

func TestModeMockCustomURL(t *testing.T) {
	const custom = "http://localhost:9000"
	setEnv(t, map[string]string{"MODE": "mock", "MOCK_URL": custom})
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.BluelinkBaseURL != custom || c.BluelinkLoginURL != custom {
		t.Errorf("mock hosts = base=%q login=%q, want %q", c.BluelinkBaseURL, c.BluelinkLoginURL, custom)
	}
}

func TestModeLiveRequiresCreds(t *testing.T) {
	setEnv(t, map[string]string{"MODE": "live", "BLUELINK_USERNAME": "", "BLUELINK_PASSWORD": ""})
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing creds in live mode")
	}
	for _, want := range []string{"BLUELINK_USERNAME", "BLUELINK_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %v", want, err)
		}
	}
}

func TestModeBogus(t *testing.T) {
	setEnv(t, map[string]string{"MODE": "bogus"})
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MODE") {
		t.Fatalf("expected MODE validation error, got %v", err)
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

func TestRejectsMalformedValues(t *testing.T) {
	cases := []struct {
		name, key, value, want string
	}{
		{"bad int threshold", "READY_FAILURE_THRESHOLD", "abc", "READY_FAILURE_THRESHOLD"},
		{"bad int retries", "POLL_MAX_RETRIES", "x", "POLL_MAX_RETRIES"},
		{"bad bool", "FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN", "yes", "FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN"},
		{"broker without scheme", "MQTT_BROKER_URL", "localhost:1883", "MQTT_BROKER_URL"},
		{"broker without host", "MQTT_BROKER_URL", "mqtt://", "MQTT_BROKER_URL"},
		{"broker path only", "MQTT_BROKER_URL", "/just/a/path", "MQTT_BROKER_URL"},
		{"bad log level", "LOG_LEVEL", "verbose", "LOG_LEVEL"},
		{"bad log format", "LOG_FORMAT", "yaml", "LOG_FORMAT"},
		{"single digit hour", "FORCE_REFRESH_AT", "5:00", "FORCE_REFRESH_AT"},
		{"single digit minute", "FORCE_REFRESH_AT", "05:0", "FORCE_REFRESH_AT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, map[string]string{tc.key: tc.value})
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for %s=%q", tc.key, tc.value)
			}
			if !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.value) {
				t.Errorf("error %q should name %s and the bad value %q", err, tc.want, tc.value)
			}
		})
	}
}

func TestAcceptsWellFormedValues(t *testing.T) {
	cases := []struct {
		name, key, value string
		check            func(*Config) bool
	}{
		{"bool true", "FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN", "true", func(c *Config) bool { return c.ForceRefreshOnlyWhenPluggedIn }},
		{"bool 1", "FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN", "1", func(c *Config) bool { return c.ForceRefreshOnlyWhenPluggedIn }},
		{"bool false", "FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN", "false", func(c *Config) bool { return !c.ForceRefreshOnlyWhenPluggedIn }},
		{"int threshold", "READY_FAILURE_THRESHOLD", "7", func(c *Config) bool { return c.ReadyFailureThreshold == 7 }},
		{"int retries", "POLL_MAX_RETRIES", "0", func(c *Config) bool { return c.PollMaxRetries == 0 }},
		{"two digit time", "FORCE_REFRESH_AT", "05:00", func(c *Config) bool { return c.ForceRefreshHour == 5 && c.ForceRefreshMinute == 0 }},
		{"late time", "FORCE_REFRESH_AT", "23:59", func(c *Config) bool { return c.ForceRefreshHour == 23 && c.ForceRefreshMinute == 59 }},
		{"mqtt broker", "MQTT_BROKER_URL", "mqtt://host:1883", func(c *Config) bool { return c.MQTTBrokerURL == "mqtt://host:1883" }},
		{"tcp broker", "MQTT_BROKER_URL", "tcp://host", func(c *Config) bool { return c.MQTTBrokerURL == "tcp://host" }},
		{"level debug", "LOG_LEVEL", "debug", func(c *Config) bool { return c.LogLevel == "debug" }},
		{"level info", "LOG_LEVEL", "info", func(c *Config) bool { return c.LogLevel == "info" }},
		{"level warn", "LOG_LEVEL", "WARN", func(c *Config) bool { return c.LogLevel == "warn" }},
		{"level error", "LOG_LEVEL", "error", func(c *Config) bool { return c.LogLevel == "error" }},
		{"format json", "LOG_FORMAT", "json", func(c *Config) bool { return c.LogFormat == "json" }},
		{"format text", "LOG_FORMAT", "Text", func(c *Config) bool { return c.LogFormat == "text" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, map[string]string{tc.key: tc.value})
			c, err := Load()
			if err != nil {
				t.Fatalf("%s=%q: unexpected error: %v", tc.key, tc.value, err)
			}
			if !tc.check(c) {
				t.Errorf("%s=%q not applied: %s", tc.key, tc.value, c)
			}
		})
	}
}

func TestBrokerURLErrorRedactsPassword(t *testing.T) {
	cases := map[string]string{
		"missing host":   "mqtt://user:hunter2@",
		"invalid port":   "mqtt://user:hunter2@host:abc",
		"percent escape": "mqtt://user:hunter2@host/%zz",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			setEnv(t, map[string]string{"MQTT_BROKER_URL": raw})
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "MQTT_BROKER_URL") {
				t.Fatalf("expected MQTT_BROKER_URL error, got %v", err)
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Errorf("error leaks broker password: %q", err)
			}
		})
	}
}
