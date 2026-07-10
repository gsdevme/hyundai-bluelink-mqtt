package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/config"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/homeassistant"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/mqtt"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/publisher"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/scheduler"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/server"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the poll -> MQTT service",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runServe(cmd.Context())
	},
}

func runServe(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := newLogger(cfg.LogLevel, cfg.LogFormat)
	logger.Info("starting", "config", cfg.String())

	// Status/health server listens immediately so probes work during init.
	status := server.New(server.Config{
		ReadyFailureThreshold: cfg.ReadyFailureThreshold,
		PollInterval:          cfg.PollInterval,
		ForceEnabled:          cfg.ForceRefreshEnabled,
		ForceHour:             cfg.ForceRefreshHour,
		ForceMinute:           cfg.ForceRefreshMinute,
		ForceLocation:         cfg.ForceRefreshLocation,
	})
	healthSrv := &http.Server{Addr: cfg.HealthAddr, Handler: status.Handler()}
	healthErr := make(chan error, 1)
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			healthErr <- err
			return
		}
		healthErr <- nil
	}()

	// Token store.
	store, err := buildTokenStore(cfg)
	if err != nil {
		return err
	}

	// Bluelink client: authenticate/restore + select target vehicle.
	client, err := bluelink.New(bluelink.Config{
		Username:  cfg.BluelinkUsername,
		Password:  cfg.BluelinkPassword,
		VIN:       cfg.BluelinkVIN,
		LoginHost: cfg.BluelinkLoginURL,
		SPABase:   cfg.BluelinkBaseURL,
		Store:     store,
		Logger:    logger,
	})
	if err != nil {
		return err
	}
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("bluelink connect: %w", err)
	}
	vehicle, err := client.SelectVehicle(ctx)
	if err != nil {
		return fmt.Errorf("select vehicle: %w", err)
	}
	logger.Info("selected vehicle", "model", vehicle.Model, "ccs2", vehicle.IsCCS2())
	status.SetVehicle(vehicle.Model, vehicle.Name, vehicle.VIN, vehicle.IsCCS2())
	if !vehicle.IsCCS2() {
		// Documented degradation: CCS1 is not implemented; idle without publishing.
		logger.Warn("target vehicle is not CCS2; status publishing is not supported, idling")
		select {
		case err := <-healthErr:
			return fmt.Errorf("health server: %w", err)
		case <-ctx.Done():
		}
		return shutdownServer(healthSrv)
	}

	// Discovery config + MQTT connection (with LWT).
	haCfg := homeassistant.Config{
		DiscoveryPrefix: cfg.HADiscoveryPrefix,
		TopicPrefix:     cfg.MQTTTopicPrefix,
		VIN:             vehicle.VIN,
		Model:           vehicle.Model,
		Name:            vehicle.Name,
	}
	mc, err := mqtt.Connect(ctx, mqtt.Options{
		BrokerURL:         cfg.MQTTBrokerURL,
		Username:          cfg.MQTTUsername,
		Password:          cfg.MQTTPassword,
		ClientID:          cfg.MQTTClientID,
		AvailabilityTopic: haCfg.AvailabilityTopic(),
		Logger:            logger,
	})
	if err != nil {
		return fmt.Errorf("mqtt: %w", err)
	}

	pub := publisher.New(mc, haCfg)
	// Republish availability + discovery on every (re)connection.
	mc.SetOnConnectionUp(func(ctx context.Context) {
		if err := pub.PublishAvailability(ctx, true); err != nil {
			logger.Warn("republish availability failed", "err", err)
		}
		if err := pub.PublishDiscovery(ctx); err != nil {
			logger.Warn("republish discovery failed", "err", err)
		}
	})
	// Initial publish (in case the first connection-up fired before the callback
	// was registered).
	if err := pub.PublishDiscovery(ctx); err != nil {
		return fmt.Errorf("publish discovery: %w", err)
	}
	if err := pub.PublishAvailability(ctx, true); err != nil {
		return fmt.Errorf("publish availability: %w", err)
	}

	// Scheduler.
	sched := scheduler.New(client, pub, status, scheduler.Config{
		Vehicle:                vehicle,
		PollInterval:           cfg.PollInterval,
		MaxRetries:             cfg.PollMaxRetries,
		ForceEnabled:           cfg.ForceRefreshEnabled,
		ForceHour:              cfg.ForceRefreshHour,
		ForceMinute:            cfg.ForceRefreshMinute,
		ForceLocation:          cfg.ForceRefreshLocation,
		ForceOnlyWhenPluggedIn: cfg.ForceRefreshOnlyWhenPluggedIn,
		Logger:                 logger,
	})

	schedDone := make(chan struct{})
	go func() { sched.Run(ctx); close(schedDone) }()

	logger.Info("service running")
	select {
	case err := <-healthErr:
		return fmt.Errorf("health server: %w", err)
	case <-ctx.Done():
	}
	logger.Info("shutting down")

	// Graceful shutdown: explicit offline, clean disconnect, stop everything.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pub.PublishAvailability(shutdownCtx, false); err != nil {
		logger.Warn("publish offline failed", "err", err)
	}
	if err := mc.Disconnect(shutdownCtx); err != nil {
		logger.Warn("mqtt disconnect failed", "err", err)
	}
	<-schedDone
	return shutdownServer(healthSrv)
}

func buildTokenStore(cfg *config.Config) (bluelink.TokenStore, error) {
	switch cfg.TokenStore {
	case "kube":
		return bluelink.NewKubeSecretStore(cfg.TokenSecretName)
	default:
		return bluelink.NewMemoryStore(), nil
	}
}

func shutdownServer(srv *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
