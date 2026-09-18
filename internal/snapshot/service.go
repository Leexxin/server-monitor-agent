package snapshot

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"sma/internal/collector"
	"sma/internal/model"
)

type Service struct {
	collectors []collector.Collector
	timeout    time.Duration
	cacheTTL   time.Duration
	hostname   string

	mu       sync.Mutex
	cached   model.Snapshot
	cachedAt time.Time
	inflight chan struct{}
}

func New(collectors []collector.Collector, timeout, cacheTTL time.Duration) *Service {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	return &Service{collectors: collectors, timeout: timeout, cacheTTL: cacheTTL, hostname: hostname}
}

func (s *Service) Get(ctx context.Context) (model.Snapshot, error) {
	s.mu.Lock()
	if !s.cachedAt.IsZero() && s.cacheTTL > 0 && time.Since(s.cachedAt) <= s.cacheTTL {
		result := s.cached
		s.mu.Unlock()
		return result, nil
	}
	if s.inflight == nil {
		s.inflight = make(chan struct{})
		go s.refresh()
	}
	done := s.inflight
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		return model.Snapshot{}, ctx.Err()
	case <-done:
		s.mu.Lock()
		result := s.cached
		s.mu.Unlock()
		return result, nil
	}
}

func (s *Service) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	now := time.Now().UTC()
	snapshot := model.Snapshot{
		SchemaVersion: model.SchemaVersion, Timestamp: now, Host: model.Host{Hostname: s.hostname},
		Filesystems: []model.Filesystem{}, Disks: []model.Disk{}, Errors: []model.CollectorError{},
		CollectorInfo: make(map[string]model.CollectorStatus, len(s.collectors)),
	}
	type outcome struct {
		name     string
		partial  model.Partial
		err      error
		duration time.Duration
	}
	results := make(chan outcome, len(s.collectors))
	pending := make(map[string]bool, len(s.collectors))
	for _, item := range s.collectors {
		pending[item.Name()] = true
		go func(c collector.Collector) {
			started := time.Now()
			var partial model.Partial
			var err error
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						err = fmt.Errorf("collector panic")
					}
				}()
				partial, err = c.Collect(ctx)
			}()
			results <- outcome{name: c.Name(), partial: partial, err: err, duration: time.Since(started)}
		}(item)
	}
	for len(pending) > 0 {
		var result outcome
		select {
		case result = <-results:
			delete(pending, result.name)
		case <-ctx.Done():
			for name := range pending {
				snapshot.CollectorInfo[name] = model.CollectorStatus{Success: false, Duration: s.timeout}
				snapshot.Errors = append(snapshot.Errors, model.CollectorError{
					Collector: name, Code: "timeout", Message: "collector timed out",
				})
			}
			pending = nil
			continue
		}
		status := model.CollectorStatus{Success: result.err == nil, Duration: result.duration}
		snapshot.CollectorInfo[result.name] = status
		snapshot.Merge(result.partial)
		if result.err != nil {
			code := "collection_failed"
			if ctx.Err() != nil {
				code = "timeout"
			}
			snapshot.Errors = append(snapshot.Errors, model.CollectorError{
				Collector: result.name, Code: code, Message: safeMessage(result.err),
			})
			continue
		}
	}
	sort.Slice(snapshot.Errors, func(i, j int) bool {
		return snapshot.Errors[i].Collector < snapshot.Errors[j].Collector
	})

	s.mu.Lock()
	s.cached = snapshot
	s.cachedAt = time.Now()
	done := s.inflight
	s.inflight = nil
	s.mu.Unlock()
	close(done)
}

func (s *Service) Ready(snapshot model.Snapshot) bool {
	for _, required := range []string{"cpu", "memory"} {
		if !containsCollector(s.collectors, required) {
			continue
		}
		if !snapshot.CollectorInfo[required].Success {
			return false
		}
	}
	return true
}

func containsCollector(collectors []collector.Collector, name string) bool {
	for _, item := range collectors {
		if item.Name() == name {
			return true
		}
	}
	return false
}

func safeMessage(err error) string {
	message := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(message) > 256 {
		return message[:256]
	}
	return message
}
