package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
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
	if err := platformMain(run); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func run(rootCtx context.Context, started func()) (result error) {
	logger, closeLog, err := platformLogger()
	if err != nil {
		return err
	}
	defer func() {
		if result != nil {
			logger.Error("agent failed", "error", result)
		}
		closeLog()
	}()
	args, err := platformArgs(os.Args[1:])
	if err != nil {
		return err
	}
	cfg, err := config.Parse(args)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		return err
	}
	service := snapshot.New(collector.Enabled(cfg), cfg.CollectorTimeout, cfg.CacheTTL)
	detectors := []discovery.Detector{discovery.ProcessDetector{ProcPath: cfg.ProcPath}}
	scriptDetectors, err := discovery.LoadScriptDetectors(cfg.DiscoveryScriptDir)
	if err != nil {
		logger.Error("load discovery scripts", "error", err)
		return err
	}
	detectors = append(detectors, scriptDetectors...)
	reportToken, err := readOptionalSecret(cfg.DiscoveryReportTokenFile)
	if err != nil {
		logger.Error("read discovery report token", "error", err)
		return err
	}
	reporter, err := discovery.NewReporter(cfg.DiscoveryReportURL, reportToken)
	if err != nil {
		logger.Error("configure discovery reporter", "error", err)
		return err
	}
	discoveryService := discovery.New(detectors, discovery.Options{
		Enabled: cfg.DiscoveryEnabled, Timeout: cfg.DiscoveryTimeout,
		Retention: cfg.DiscoveryRetention, Reporter: reporter,
	})
	webServer, err := web.New(cfg, service, discoveryService, buildinfo.Current())
	if err != nil {
		logger.Error("initialize HTTP server", "error", err)
		return err
	}
	httpServer := webServer.HTTPServer()
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		logger.Error("listen failed", "error", err)
		return err
	}
	defer listener.Close()

	rootCtx, stop := signal.NotifyContext(rootCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	started()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server monitor agent started", "address", cfg.ListenAddress, "version", buildinfo.Version)
		if cfg.TLSCertFile != "" {
			serverErrors <- httpServer.ServeTLS(listener, cfg.TLSCertFile, cfg.TLSKeyFile)
			return
		}
		serverErrors <- httpServer.Serve(listener)
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("shutdown requested")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			return err
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		return err
	}
	logger.Info("server monitor agent stopped")
	return nil
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
