package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"sma/internal/buildinfo"
	"sma/internal/collector"
	"sma/internal/config"
	"sma/internal/snapshot"
	"sma/internal/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(2)
	}
	service := snapshot.New(collector.Enabled(cfg), cfg.CollectorTimeout, cfg.CacheTTL)
	webServer, err := web.New(cfg, service, buildinfo.Current())
	if err != nil {
		logger.Error("initialize HTTP server", "error", err)
		os.Exit(2)
	}
	httpServer := webServer.HTTPServer()

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server monitor agent started", "address", cfg.ListenAddress, "version", buildinfo.Version)
		if cfg.TLSCertFile != "" {
			serverErrors <- httpServer.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
			return
		}
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("shutdown requested")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server monitor agent stopped")
}
