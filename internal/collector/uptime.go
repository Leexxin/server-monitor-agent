package collector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sma/internal/model"
)

type UptimeCollector struct{ ProcPath string }

func (UptimeCollector) Name() string { return "uptime" }

func (c UptimeCollector) Collect(_ context.Context) (model.Partial, error) {
	data, err := os.ReadFile(filepath.Join(c.ProcPath, "uptime"))
	if err != nil {
		return model.Partial{}, fmt.Errorf("read uptime: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return model.Partial{}, fmt.Errorf("parse uptime: empty input")
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || uptime < 0 {
		return model.Partial{}, fmt.Errorf("parse uptime: invalid value %q", fields[0])
	}
	boot := float64(time.Now().UnixNano())/1e9 - uptime
	return model.Partial{Uptime: &uptime, BootTime: &boot}, nil
}
