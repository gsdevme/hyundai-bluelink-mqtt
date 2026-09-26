// Package config binds and validates the service configuration from environment
// variables. See docs/specs/05-config.md.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// MinPollInterval is the floor for POLL_INTERVAL, protecting against rate limits
// and accidental car-wake pressure.
const MinPollInterval = 5 * time.Minute

// defaultMockURL is the mock target used when MODE=mock and MOCK_URL is unset.
// Must match the mock command's default --addr :8090 (see internal/cmd/mock.go).
const defaultMockURL = "http://localhost:8090"

// Config is the fully-parsed, validated service configuration.
type Config struct {
	// Bluelink
	BluelinkUsername string
	BluelinkPassword string
	BluelinkVIN      string
	BluelinkPIN      string
	Mode             string // "live" | "mock"; resolves the Bluelink targets below
	BluelinkBaseURL  string // resolved SPA host: empty in live, MOCK_URL in mock
	BluelinkLoginURL string // resolved login host: empty in live, MOCK_URL in mock

	// Polling & scheduling
	PollInterval                  time.Duration
	ForceRefreshEnabled           bool
	ForceRefreshHour              int
	ForceRefreshMinute            int
	ForceRefreshLocation          *time.Location
	ForceRefreshOnlyWhenPluggedIn bool
	ReadyFailureThreshold         int
	PollMaxRetries                int

	// MQTT / HA
	MQTTBrokerURL     string
	MQTTUsername      string
	MQTTPassword      string
	MQTTClientID      string
	MQTTTopicPrefix   string
	HADiscoveryPrefix string
	DistanceUnit      string // "km" | "mi"; value + HA label for both range and odometer

	// Tokens
	TokenStore      string // "memory" | "kube"
	TokenSecretName string

	// Health & logging
	HealthAddr string
	LogLevel   string
	LogFormat  string
}

// Load reads configuration from the environment, applies defaults and validates.
func Load() (*Config, error) {
	c := &Config{
		BluelinkUsername:  os.Getenv("BLUELINK_USERNAME"),
		BluelinkPassword:  os.Getenv("BLUELINK_PASSWORD"),
		BluelinkVIN:       os.Getenv("BLUELINK_VIN"),
		BluelinkPIN:       os.Getenv("BLUELINK_PIN"),
		MQTTBrokerURL:     os.Getenv("MQTT_BROKER_URL"),
		MQTTUsername:      os.Getenv("MQTT_USERNAME"),
		MQTTPassword:      os.Getenv("MQTT_PASSWORD"),
		MQTTClientID:      getEnv("MQTT_CLIENT_ID", "hyundai-bluelink-mqtt"),
		MQTTTopicPrefix:   getEnv("MQTT_TOPIC_PREFIX", "hyundai_bluelink"),
		HADiscoveryPrefix: getEnv("HA_DISCOVERY_PREFIX", "homeassistant"),
		DistanceUnit:      strings.ToLower(getEnv("DISTANCE_UNIT", "km")),
		TokenStore:        strings.ToLower(getEnv("TOKEN_STORE", "memory")),
		TokenSecretName:   os.Getenv("TOKEN_SECRET_NAME"),
		HealthAddr:        getEnv("HEALTH_ADDR", ":8080"),
		LogLevel:          strings.ToLower(getEnv("LOG_LEVEL", "info")),
		LogFormat:         strings.ToLower(getEnv("LOG_FORMAT", "json")),
	}

	var errs []error

	// Resolve the target: live uses the real Hyundai hosts (empty overrides),
	// mock points both hosts at the local mock server.
	c.Mode = strings.ToLower(getEnv("MODE", "live"))
	switch c.Mode {
	case "live":
		// leave Bluelink hosts empty -> bluelink.New() uses the real Hyundai hosts
	case "mock":
		mockURL := getEnv("MOCK_URL", defaultMockURL)
		c.BluelinkBaseURL, c.BluelinkLoginURL = mockURL, mockURL
		// mock ignores credentials; supply dummies so bluelink.New() doesn't reject them
		if c.BluelinkUsername == "" {
			c.BluelinkUsername = "mock"
		}
		if c.BluelinkPassword == "" {
			c.BluelinkPassword = "mock"
		}
	default:
		errs = append(errs, fmt.Errorf("MODE must be live or mock, got %q", c.Mode))
	}

	// Credentials are only required against the real API; mock supplies dummies above.
	if c.Mode == "live" {
		if c.BluelinkUsername == "" {
			errs = append(errs, errors.New("BLUELINK_USERNAME is required"))
		}
		if c.BluelinkPassword == "" {
			errs = append(errs, errors.New("BLUELINK_PASSWORD is required"))
		}
	}
	if c.MQTTBrokerURL == "" {
		errs = append(errs, errors.New("MQTT_BROKER_URL is required"))
	} else if err := validateBrokerURL(c.MQTTBrokerURL); err != nil {
		errs = append(errs, err)
	}

	interval, err := parseDuration("POLL_INTERVAL", 45*time.Minute)
	if err != nil {
		errs = append(errs, err)
	} else if interval < MinPollInterval {
		errs = append(errs, fmt.Errorf("POLL_INTERVAL %s is below the %s floor", interval, MinPollInterval))
	}
	c.PollInterval = interval

	if c.ReadyFailureThreshold, err = getInt("READY_FAILURE_THRESHOLD", 3); err != nil {
		errs = append(errs, err)
	} else if c.ReadyFailureThreshold < 1 {
		errs = append(errs, errors.New("READY_FAILURE_THRESHOLD must be >= 1"))
	}
	if c.PollMaxRetries, err = getInt("POLL_MAX_RETRIES", 3); err != nil {
		errs = append(errs, err)
	} else if c.PollMaxRetries < 0 {
		errs = append(errs, errors.New("POLL_MAX_RETRIES must be >= 0"))
	}
	if c.ForceRefreshOnlyWhenPluggedIn, err = getBool("FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN", false); err != nil {
		errs = append(errs, err)
	}

	if err := c.parseForceRefresh(); err != nil {
		errs = append(errs, err)
	}

	switch c.DistanceUnit {
	case "km", "mi":
	default:
		errs = append(errs, fmt.Errorf("DISTANCE_UNIT must be km or mi, got %q", c.DistanceUnit))
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be debug, info, warn or error, got %q", c.LogLevel))
	}

	switch c.LogFormat {
	case "json", "text":
	default:
		errs = append(errs, fmt.Errorf("LOG_FORMAT must be json or text, got %q", c.LogFormat))
	}

	switch c.TokenStore {
	case "memory":
	case "kube":
		if c.TokenSecretName == "" {
			errs = append(errs, errors.New("TOKEN_SECRET_NAME is required when TOKEN_STORE=kube"))
		}
	default:
		errs = append(errs, fmt.Errorf("TOKEN_STORE must be memory or kube, got %q", c.TokenStore))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

// parseForceRefresh resolves FORCE_REFRESH_AT/FORCE_REFRESH_TZ. An empty
// FORCE_REFRESH_AT disables the daily refresh; otherwise it must be a strict
// two-digit HH:MM, since the "15:04" layout alone also accepts "5:00".
func (c *Config) parseForceRefresh() error {
	at := getEnv("FORCE_REFRESH_AT", "05:00")
	if at == "" {
		c.ForceRefreshEnabled = false
		return nil
	}
	t, err := time.Parse("15:04", at)
	if err != nil || len(at) != len("15:04") {
		return fmt.Errorf("FORCE_REFRESH_AT must be HH:MM, got %q", at)
	}
	tzName := getEnv("FORCE_REFRESH_TZ", "UTC")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return fmt.Errorf("FORCE_REFRESH_TZ %q is not a valid timezone: %w", tzName, err)
	}
	c.ForceRefreshEnabled = true
	c.ForceRefreshHour = t.Hour()
	c.ForceRefreshMinute = t.Minute()
	c.ForceRefreshLocation = loc
	return nil
}

// String renders the config with secrets redacted (safe to log).
func (c *Config) String() string {
	mode := c.Mode
	if c.Mode == "mock" {
		mode = fmt.Sprintf("mock(%s)", c.BluelinkBaseURL)
	}
	return fmt.Sprintf("Config{mode=%s user=%s vin=%s poll=%s forceRefresh=%v@%02d:%02d %s pluggedGate=%v "+
		"broker=%s topicPrefix=%s haPrefix=%s distanceUnit=%s tokenStore=%s secret=%s health=%s log=%s/%s}",
		mode, c.BluelinkUsername, redact(c.BluelinkVIN), c.PollInterval, c.ForceRefreshEnabled,
		c.ForceRefreshHour, c.ForceRefreshMinute, locName(c.ForceRefreshLocation), c.ForceRefreshOnlyWhenPluggedIn,
		c.MQTTBrokerURL, c.MQTTTopicPrefix, c.HADiscoveryPrefix, c.DistanceUnit, c.TokenStore, c.TokenSecretName,
		c.HealthAddr, c.LogLevel, c.LogFormat)
}

func locName(l *time.Location) string {
	if l == nil {
		return "-"
	}
	return l.String()
}

func redact(s string) string {
	if s == "" {
		return ""
	}
	return "***"
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// validateBrokerURL requires an absolute URL with both a scheme and a host, so
// that a bare "host:port" (which url.Parse reads as scheme "host") is rejected.
// The raw URL never appears in the error: url.Parse failures are reported by
// their inner cause, since *url.Error formats the unredacted input, and the
// scheme/host check prints the redacted form.
func validateBrokerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("MQTT_BROKER_URL is invalid: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("MQTT_BROKER_URL must include a scheme and host (e.g. mqtt://host:1883), got %q", u.Redacted())
	}
	return nil
}

// getBool returns def when key is unset, otherwise the strconv.ParseBool value;
// an unparseable value is an error naming the variable.
func getBool(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean (true/false/1/0), got %q", key, v)
	}
	return b, nil
}

// getInt returns def when key is unset, otherwise the strconv.Atoi value; an
// unparseable value is an error naming the variable.
func getInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, v)
	}
	return n, nil
}

func parseDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid duration: %w", key, err)
	}
	return d, nil
}
