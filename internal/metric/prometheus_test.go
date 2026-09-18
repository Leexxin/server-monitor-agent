package metric

import (
	"strings"
	"testing"
	"time"

	"sma/internal/buildinfo"
	"sma/internal/model"
)

func TestPrometheusEncoding(t *testing.T) {
	snapshot := model.Snapshot{
		CPU:           &model.CPU{LogicalCount: 1, Seconds: map[string]map[string]float64{"cpu0": {"idle": 12.5}}},
		Filesystems:   []model.Filesystem{{Device: `/dev/a"b`, Mountpoint: "/data\nline", FilesystemType: "ext4", SizeBytes: 42}},
		CollectorInfo: map[string]model.CollectorStatus{"cpu": {Success: true, Duration: time.Millisecond}},
	}
	result := string(Prometheus(snapshot, buildinfo.Info{Version: "test", Revision: "abc", GoVersion: "go-test"}, AgentStats{Scrapes: 2}))
	for _, expected := range []string{
		`# TYPE sma_cpu_seconds_total counter`,
		`sma_cpu_seconds_total{cpu="cpu0",mode="idle"} 12.5`,
		`sma_filesystem_size_bytes{device="/dev/a\"b",fstype="ext4",mountpoint="/data\nline"} 42`,
		`sma_collector_success{collector="cpu"} 1`,
	} {
		if !strings.Contains(result, expected) {
			t.Errorf("output does not contain %q:\n%s", expected, result)
		}
	}
}
