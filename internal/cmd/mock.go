package cmd

import (
	"errors"
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

		serveErr := make(chan error, 1)
		go func() {
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr <- err
				return
			}
			serveErr <- nil
		}()

		select {
		case err := <-serveErr:
			return err
		case <-cmd.Context().Done():
			logger.Info("shutting down mock")
			if err := shutdownServer(httpSrv); err != nil {
				return err
			}
			// Drain the serve goroutine so a serve error racing the signal
			// isn't discarded; it always sends exactly once.
			return <-serveErr
		}
	},
}

func init() {
	mockCmd.Flags().StringVar(&mockAddr, "addr", ":8090", "listen address for the mock API")
}
