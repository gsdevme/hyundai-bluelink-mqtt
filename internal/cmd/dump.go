package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/bluelink"
	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/config"
)

var dumpForce bool

// dumpCmd is a hidden diagnostic: it GETs the candidate CCS1 endpoints and
// pretty-prints the raw JSON so responses can be captured as test fixtures when
// adding protocol support. It never publishes anything.
var dumpCmd = &cobra.Command{
	Use:    "dump",
	Short:  "Capture raw Bluelink API responses for the selected vehicle (diagnostic)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDump(cmd.Context(), dumpForce)
	},
}

func init() {
	dumpCmd.Flags().BoolVar(&dumpForce, "force", false,
		"also GET the force-status endpoint, which wakes the car")
}

func runDump(ctx context.Context, force bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := newLogger(cfg.LogLevel, cfg.LogFormat)

	store, err := buildTokenStore(cfg)
	if err != nil {
		return err
	}

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

	// Cached endpoints first (no wake); the force status endpoint wakes the car
	// and is only reached with --force.
	paths := []string{
		"vehicles/" + vehicle.ID + "/status/latest",
		"vehicles/" + vehicle.ID + "/location",
		"vehicles/" + vehicle.ID + "/location/park",
	}
	if force {
		paths = append(paths, "vehicles/"+vehicle.ID+"/status")
	}

	for _, p := range paths {
		dumpPath(ctx, client, vehicle, p)
	}
	return nil
}

// dumpPath GETs one path and prints it, never failing the whole run so a single
// unsupported endpoint does not hide the others.
func dumpPath(ctx context.Context, client *bluelink.Client, v bluelink.Vehicle, path string) {
	fmt.Printf("=== %s ===\n", path)
	body, err := client.DebugGet(ctx, v, path)
	if err != nil {
		fmt.Printf("error: %v\n\n", err)
		return
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		// Not valid JSON; print the raw body so it is still visible.
		fmt.Printf("%s\n\n", body)
		return
	}
	fmt.Printf("%s\n\n", pretty.String())
}
