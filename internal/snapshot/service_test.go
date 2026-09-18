package snapshot

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"sma/internal/collector"
	"sma/internal/model"
)

type testCollector struct {
	name          string
	calls         *atomic.Int32
	wait          time.Duration
	ignoreContext bool
}

func (c testCollector) Name() string { return c.name }
func (c testCollector) Collect(ctx context.Context) (model.Partial, error) {
	c.calls.Add(1)
	if c.ignoreContext {
		time.Sleep(c.wait)
		return model.Partial{}, nil
	}
	select {
	case <-time.After(c.wait):
		return model.Partial{CPU: &model.CPU{LogicalCount: 1}}, nil
	case <-ctx.Done():
		return model.Partial{}, ctx.Err()
	}
}

func TestServiceEnforcesTimeout(t *testing.T) {
	var calls atomic.Int32
	service := New([]collector.Collector{testCollector{name: "cpu", calls: &calls, wait: 200 * time.Millisecond, ignoreContext: true}}, 10*time.Millisecond, 0)
	started := time.Now()
	value, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("service waited for a collector after its timeout")
	}
	if len(value.Errors) != 1 || value.Errors[0].Code != "timeout" {
		t.Fatalf("unexpected errors: %#v", value.Errors)
	}
}

func TestServiceSharesConcurrentCollection(t *testing.T) {
	var calls atomic.Int32
	service := New([]collector.Collector{testCollector{name: "cpu", calls: &calls, wait: 10 * time.Millisecond}}, time.Second, time.Second)
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			if _, err := service.Get(context.Background()); err != nil {
				t.Error(err)
			}
			done <- struct{}{}
		}()
	}
	<-done
	<-done
	if calls.Load() != 1 {
		t.Fatalf("collector called %d times", calls.Load())
	}
}
