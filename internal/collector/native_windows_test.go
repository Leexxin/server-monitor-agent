package collector

import (
	"context"
	"testing"
	"unsafe"

	"sma/internal/config"
)

func TestNativeWindowsCollectors(t *testing.T) {
	if unsafe.Sizeof(memoryStatus{}) != 64 || unsafe.Sizeof(diskPerformance{}) != 88 {
		t.Fatal("Windows ABI structure size mismatch")
	}
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EnabledCollectors["load"] || cfg.DiscoveryEnabled {
		t.Fatal("Windows must not enable unsupported features by default")
	}
	for _, c := range Enabled(cfg) {
		p, err := c.Collect(context.Background())
		if err != nil && c.Name() == "disk" {
			t.Logf("disk driver limitation (verified by lifecycle separately): %v", err)
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.Name(), err)
		}
		switch c.Name() {
		case "cpu":
			if p.CPU == nil || p.CPU.LogicalCount < 1 || p.CPU.Seconds["total"]["idle"] < 0 {
				t.Fatal("invalid CPU")
			}
		case "memory":
			if p.Memory == nil || p.Memory.TotalBytes == 0 || p.Memory.AvailableBytes > p.Memory.TotalBytes {
				t.Fatal("invalid memory")
			}
		case "filesystem":
			if len(p.Filesystems) == 0 || p.Filesystems[0].SizeBytes == 0 {
				t.Fatal("no fixed volume statistics")
			}
		case "uptime":
			if p.Uptime == nil || *p.Uptime <= 0 {
				t.Fatal("invalid uptime")
			}
		}
	}
}
