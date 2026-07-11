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
		BluelinkUsername:              os.Getenv("BLUELINK_USERNAME"),
		BluelinkPassword:              os.Getenv("BLUELINK_PASSWORD"),
		BluelinkVIN:                   os.Getenv("BLUELINK_VIN"),
		BluelinkPIN:                   os.Getenv("BLUELINK_PIN"),
		ForceRefreshOnlyWhenPluggedIn: getBool("FORCE_REFRESH_ONLY_WHEN_PLUGGED_IN", false),
		MQTTBrokerURL:                 os.Getenv("MQTT_BROKER_URL"),
		MQTTUsername:                  os.Getenv("MQTT_USERNAME"),
		MQTTPassword:                  os.Getenv("MQTT_PASSWORD"),
		MQTTClientID:                  getEnv("MQTT_CLIENT_ID", "hyundai-bluelink-mqtt"),
		MQTTTopicPrefix:               getEnv("MQTT_TOPIC_PREFIX", "hyundai_bluelink"),
		HADiscoveryPrefix:             getEnv("HA_DISCOVERY_PREFIX", "homeassistant"),
		TokenStore:                    strings.ToLower(getEnv("TOKEN_STORE", "memory")),
		TokenSecretName:               os.Getenv("TOKEN_SECRET_NAME"),
		HealthAddr:                    getEnv("HEALTH_ADDR", ":8080"),
		LogLevel:                      strings.ToLower(getEnv("LOG_LEVEL", "info")),
		LogFormat:                     strings.ToLower(getEnv("LOG_FORMAT", "json")),
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
	} else if _, err := url.Parse(c.MQTTBrokerURL); err != nil {
		errs = append(errs, fmt.Errorf("MQTT_BROKER_URL is invalid: %w", err))
	}

	interval, err := parseDuration("POLL_INTERVAL", 30*time.Minute)
	if err != nil {
		errs = append(errs, err)
	} else if interval < MinPollInterval {
		errs = append(errs, fmt.Errorf("POLL_INTERVAL %s is below the %s floor", interval, MinPollInterval))
	}
	c.PollInterval = interval

	c.ReadyFailureThreshold = getInt("READY_FAILURE_THRESHOLD", 3)
	if c.ReadyFailureThreshold < 1 {
		errs = append(errs, errors.New("READY_FAILURE_THRESHOLD must be >= 1"))
	}
	c.PollMaxRetries = getInt("POLL_MAX_RETRIES", 3)
	if c.PollMaxRetries < 0 {
		errs = append(errs, errors.New("POLL_MAX_RETRIES must be >= 0"))
	}

	if err := c.parseForceRefresh(); err != nil {
		errs = append(errs, err)
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

func (c *Config) parseForceRefresh() error {
	at := getEnv("FORCE_REFRESH_AT", "05:00")
	if at == "" {
		c.ForceRefreshEnabled = false
		return nil
	}
	t, err := time.Parse("15:04", at)
	if err != nil {
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
		"broker=%s topicPrefix=%s haPrefix=%s tokenStore=%s secret=%s health=%s log=%s/%s}",
		mode, c.BluelinkUsername, redact(c.BluelinkVIN), c.PollInterval, c.ForceRefreshEnabled,
		c.ForceRefreshHour, c.ForceRefreshMinute, locName(c.ForceRefreshLocation), c.ForceRefreshOnlyWhenPluggedIn,
		c.MQTTBrokerURL, c.MQTTTopicPrefix, c.HADiscoveryPrefix, c.TokenStore, c.TokenSecretName,
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

func getBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
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
