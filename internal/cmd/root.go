// Package cmd wires the CLI: `serve` runs the poll->MQTT service, `mock` runs
// the standalone Bluelink mock server.
package cmd

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	// tzdata is embedded so time.LoadLocation works in the distroless image.
	_ "time/tzdata"
)

var rootCmd = &cobra.Command{
	Use:   "hyundai-bluelink-mqtt",
	Short: "Publish Hyundai Bluelink EV metrics to MQTT with Home Assistant autodiscovery",
	// Handle error reporting and exit codes in main() instead: don't let cobra
	// print the error or dump usage on runtime failures.
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		// Load a local .env if present (no-op in production).
		_ = godotenv.Load()
	},
}

// Execute runs the root command, returning any error for main to report and to
// map to a non-zero exit code.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(mockCmd)
}

// newLogger builds a slog logger from level/format strings.
func newLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(h)
}
