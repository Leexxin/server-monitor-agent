package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sma/internal/buildinfo"
	"sma/internal/collector"
	"sma/internal/config"
	"sma/internal/model"
	"sma/internal/snapshot"
)

type healthyCollector struct{ name string }

func (c healthyCollector) Name() string { return c.name }
func (c healthyCollector) Collect(context.Context) (model.Partial, error) {
	switch c.name {
	case "cpu":
		return model.Partial{CPU: &model.CPU{LogicalCount: 1, Seconds: map[string]map[string]float64{"cpu0": {"idle": 1}}}}, nil
	case "memory":
		return model.Partial{Memory: &model.Memory{TotalBytes: 1024, AvailableBytes: 512}}, nil
	default:
		return model.Partial{}, nil
	}
}

func TestEndpoints(t *testing.T) {
	service := snapshot.New([]collector.Collector{healthyCollector{"cpu"}, healthyCollector{"memory"}}, time.Second, time.Second)
	server, err := New(testConfig(), service, buildinfo.Info{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/healthz", 200, `"status":"ok"`},
		{"/readyz", 200, `"status":"ready"`},
		{"/version", 200, `"version":"test"`},
		{"/v1/snapshot", 200, `"schemaVersion":"v1"`},
		{"/metrics", 200, "sma_memory_total_bytes 1024"},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, req)
		if res.Code != test.status || !strings.Contains(res.Body.String(), test.contains) {
			t.Errorf("%s returned %d %q", test.path, res.Code, res.Body.String())
		}
	}
}

func TestBearerAuthentication(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.AuthTokenFile = tokenFile
	service := snapshot.New([]collector.Collector{healthyCollector{"cpu"}}, time.Second, time.Second)
	server, err := New(cfg, service, buildinfo.Info{})
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	authorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(authorized, req)
	if authorized.Code != http.StatusOK {
		t.Fatalf("status = %d", authorized.Code)
	}

	health := httptest.NewRecorder()
	server.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}
}

func testConfig() config.Config {
	return config.Config{ListenAddress: "127.0.0.1:0", TelemetryPath: "/metrics", MaxConcurrentRequests: 8}
}
