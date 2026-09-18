package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessDetectorFindsCommonServices(t *testing.T) {
	proc := t.TempDir()
	writeTestFile(t, filepath.Join(proc, "101", "comm"), []byte("nginx\n"))
	writeTestFile(t, filepath.Join(proc, "101", "cmdline"), []byte("nginx\x00-g\x00daemon off;"))
	writeTestFile(t, filepath.Join(proc, "202", "comm"), []byte("java\n"))
	writeTestFile(t, filepath.Join(proc, "202", "cmdline"), []byte("java\x00org.apache.catalina.startup.Bootstrap"))
	writeTestFile(t, filepath.Join(proc, "net", "tcp"), []byte("sl local_address rem_address st\n0: 00000000:0050 00000000:0000 0A\n"))
	sshConfig := filepath.Join(t.TempDir(), "sshd_config")
	writeTestFile(t, sshConfig, []byte("Subsystem sftp internal-sftp\n"))

	detector := ProcessDetector{ProcPath: proc, SSHConfigPath: sshConfig}
	assets, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	products := make(map[string]Asset)
	for _, asset := range assets {
		products[asset.Product] = asset
	}
	if len(products) != 3 {
		t.Fatalf("unexpected assets: %#v", assets)
	}
	if len(products["nginx"].Ports) != 1 || products["nginx"].Ports[0].Port != 80 {
		t.Fatalf("nginx port was not discovered: %#v", products["nginx"])
	}
	if products["tomcat"].Status != "running" || products["openssh-sftp"].Status != "configured" {
		t.Fatalf("unexpected process/config status: %#v", products)
	}
}

func TestParseScriptOutput(t *testing.T) {
	input := bytes.NewReader([]byte("database\tpostgresql\tPostgreSQL\thigh\tservice:postgresql.service\n"))
	assets, err := parseScriptOutput(input, "script:common-services")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Product != "postgresql" || assets[0].Evidence[0].Source != "script:common-services" {
		t.Fatalf("unexpected assets: %#v", assets)
	}
}

func TestServiceTriggersAndReports(t *testing.T) {
	received := make(chan ReportPayload, 1)
	reporter := &Reporter{endpoint: "https://receiver.invalid/discovery", token: "report-secret"}
	reporter.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer report-secret" {
			t.Errorf("missing report token")
		}
		var payload ReportPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		received <- payload
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	service := New([]Detector{staticDetector{}}, Options{Enabled: true, Timeout: time.Second, Retention: 2, Reporter: reporter})
	report := true
	run, err := service.Trigger(TriggerRequest{Report: &report})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		current, err := service.Get(run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status != StatusQueued && current.Status != StatusRunning {
			if current.Status != StatusSucceeded || current.Report.Status != "delivered" || len(current.Result.Assets) != 1 {
				t.Fatalf("unexpected completed run: %#v", current)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("discovery run did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case payload := <-received:
		if payload.RunID != run.ID || payload.Result.Assets[0].Product != "nginx" {
			t.Fatalf("unexpected report payload: %#v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("report was not delivered")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type staticDetector struct{}

func (staticDetector) Name() string { return "static" }
func (staticDetector) Detect(context.Context) ([]Asset, error) {
	return []Asset{{
		Category: "middleware", Product: "nginx", DisplayName: "Nginx", Status: "running", Confidence: "high",
		Ports: []Port{}, Evidence: []Evidence{{Source: "test", Value: "nginx"}},
	}}, nil
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScriptOutputRejectsUnknownCategory(t *testing.T) {
	_, err := parseScriptOutput(bytes.NewReader([]byte("unknown\tx\tX\thigh\tevidence\n")), "script:test")
	if err == nil || !strings.Contains(err.Error(), "invalid category") {
		t.Fatalf("expected category error, got %v", err)
	}
}
