// Package mqtt provides an autopaho-backed MQTT5 client that satisfies
// publisher.Publisher. It configures the Last Will (retained "offline" on the
// availability topic) and auto-reconnects.
package mqtt

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

// Options configure the MQTT connection.
type Options struct {
	BrokerURL         string
	Username          string
	Password          string
	ClientID          string
	AvailabilityTopic string // LWT topic; retained "offline" on unexpected loss
	Logger            *slog.Logger

	// OnConnectionUp is invoked (in a goroutine) each time the connection is
	// (re)established, so callers can (re)publish availability + discovery. It
	// may block.
	OnConnectionUp func(ctx context.Context)
}

// Client wraps an autopaho ConnectionManager and implements publisher.Publisher.
type Client struct {
	cm     *autopaho.ConnectionManager
	logger *slog.Logger
}

// Connect establishes the MQTT connection (with LWT) and waits for it to be up.
func Connect(ctx context.Context, opts Options) (*Client, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	u, err := url.Parse(opts.BrokerURL)
	if err != nil {
		return nil, fmt.Errorf("parse MQTT broker URL: %w", err)
	}

	cfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{u},
		KeepAlive:                     20,
		CleanStartOnInitialConnection: false,
		ConnectUsername:               opts.Username,
		ConnectPassword:               []byte(opts.Password),
		OnConnectError: func(err error) {
			opts.Logger.Warn("mqtt connection attempt failed", "err", err)
		},
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			opts.Logger.Info("mqtt connected")
			if opts.OnConnectionUp != nil {
				// Must not block the paho callback; run in a goroutine.
				go opts.OnConnectionUp(ctx)
			}
		},
		ClientConfig: paho.ClientConfig{ClientID: opts.ClientID},
	}
	if opts.AvailabilityTopic != "" {
		cfg.SetWillMessage(opts.AvailabilityTopic, []byte("offline"), 1, true)
	}

	cm, err := autopaho.NewConnection(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("mqtt new connection: %w", err)
	}
	if err := cm.AwaitConnection(ctx); err != nil {
		return nil, fmt.Errorf("mqtt await connection: %w", err)
	}
	return &Client{cm: cm, logger: opts.Logger}, nil
}

// Publish sends a message at QoS 1 with the given retain flag.
func (c *Client) Publish(ctx context.Context, topic string, payload []byte, retain bool) error {
	_, err := c.cm.Publish(ctx, &paho.Publish{
		QoS:     1,
		Topic:   topic,
		Payload: payload,
		Retain:  retain,
	})
	if err != nil {
		return fmt.Errorf("mqtt publish %s: %w", topic, err)
	}
	return nil
}

// Disconnect closes the connection cleanly (suppressing the LWT).
func (c *Client) Disconnect(ctx context.Context) error {
	return c.cm.Disconnect(ctx)
}
