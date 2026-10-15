package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"

	"golang.org/x/sys/windows/svc"
	"sma/internal/winagent"
)

var serviceMode bool

func platformMain(run func(context.Context, func()) error) error {
	var err error
	serviceMode, err = svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !serviceMode {
		return run(context.Background(), func() {})
	}
	return svc.Run(winagent.ServiceName, &windowsService{run: run})
}
func platformArgs(args []string) ([]string, error) {
	if !serviceMode {
		return args, nil
	}
	c, err := winagent.ReadConfig()
	if err != nil {
		return nil, err
	}
	return []string{"--listen-address=" + c.ListenAddress, "--web.auth-token-file=" + c.AuthTokenFile, "--discovery.enabled=false"}, nil
}
func platformLogger() (*slog.Logger, func(), error) {
	if !serviceMode {
		return slog.New(slog.NewJSONHandler(os.Stderr, nil)), func() {}, nil
	}
	f, err := os.OpenFile(winagent.LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, err
	}
	w := &rotatingLog{file: f}
	logger := slog.New(slog.NewJSONHandler(w, nil))
	slog.SetDefault(logger)
	return logger, func() { w.mu.Lock(); defer w.mu.Unlock(); _ = w.file.Close() }, nil
}

// Bound service logs to 10 MiB plus one 10 MiB backup.
type rotatingLog struct {
	mu   sync.Mutex
	file *os.File
}

func (w *rotatingLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if info, err := w.file.Stat(); err == nil && info.Size()+int64(len(p)) > 10<<20 {
		if err := w.file.Close(); err != nil {
			return 0, err
		}
		_ = os.Remove(winagent.LogPath() + ".1")
		if err := os.Rename(winagent.LogPath(), winagent.LogPath()+".1"); err != nil {
			return 0, err
		}
		f, err := os.OpenFile(winagent.LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return 0, err
		}
		w.file = f
	}
	return w.file.Write(p)
}

var _ io.Writer = (*rotatingLog)(nil)

type windowsService struct {
	run func(context.Context, func()) error
}

func (s *windowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending, WaitHint: 15000}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- s.run(ctx, func() { close(ready) }) }()
	current := svc.Status{State: svc.StartPending, WaitHint: 15000}
	for {
		select {
		case <-ready:
			ready = nil
			current = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
			statuses <- current
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- current
			case svc.Stop, svc.Shutdown:
				current = svc.Status{State: svc.StopPending, WaitHint: 15000}
				statuses <- current
				cancel()
			}
		case err := <-done:
			if err != nil {
				slog.Error("Windows service failed", "error", err)
				return true, 1
			}
			return false, 0
		}
	}
}
