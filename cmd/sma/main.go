package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"sma/internal/buildinfo"
	"sma/internal/collector"
	"sma/internal/config"
	"sma/internal/discovery"
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
	detectors := []discovery.Detector{discovery.ProcessDetector{ProcPath: cfg.ProcPath}}
	scriptDetectors, err := discovery.LoadScriptDetectors(cfg.DiscoveryScriptDir)
	if err != nil {
		logger.Error("load discovery scripts", "error", err)
		os.Exit(2)
	}
	detectors = append(detectors, scriptDetectors...)
	reportToken, err := readOptionalSecret(cfg.DiscoveryReportTokenFile)
	if err != nil {
		logger.Error("read discovery report token", "error", err)
		os.Exit(2)
	}
	reporter, err := discovery.NewReporter(cfg.DiscoveryReportURL, reportToken)
	if err != nil {
		logger.Error("configure discovery reporter", "error", err)
		os.Exit(2)
	}
	discoveryService := discovery.New(detectors, discovery.Options{
		Enabled: cfg.DiscoveryEnabled, Timeout: cfg.DiscoveryTimeout,
		Retention: cfg.DiscoveryRetention, Reporter: reporter,
	})
	webServer, err := web.New(cfg, service, discoveryService, buildinfo.Current())
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

func readOptionalSecret(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return "", err
	}
	if len(data) > 4096 {
		return "", fmt.Errorf("secret exceeds 4096 bytes")
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("secret file is empty")
	}
	return value, nil
}
