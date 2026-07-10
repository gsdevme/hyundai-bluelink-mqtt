package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/gsdevme/hyundai-bluelink-mqtt/internal/mock"
)

var mockAddr string

var mockCmd = &cobra.Command{
	Use:   "mock",
	Short: "Run the standalone Bluelink EU mock API server",
	Long: "Serves canned Inster CCS2 responses so the full pipeline can run " +
		"without the real Bluelink API. Point BLUELINK_BASE_URL and " +
		"BLUELINK_LOGIN_URL at this server's address.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := newLogger("info", "text")
		opts := mock.Defaults()
		opts.Logger = logger
		srv, err := mock.New(opts)
		if err != nil {
			return fmt.Errorf("build mock: %w", err)
		}
		logger.Info("mock Bluelink API listening", "addr", mockAddr)
		httpSrv := &http.Server{Addr: mockAddr, Handler: srv.Handler()}
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	},
}

func init() {
	mockCmd.Flags().StringVar(&mockAddr, "addr", ":8090", "listen address for the mock API")
}
