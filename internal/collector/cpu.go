package collector

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sma/internal/model"
)

const linuxUserHZ = 100.0

var cpuModes = []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq", "steal"}

type CPUCollector struct{ ProcPath string }

func (CPUCollector) Name() string { return "cpu" }

func (c CPUCollector) Collect(_ context.Context) (model.Partial, error) {
	f, err := os.Open(filepath.Join(c.ProcPath, "stat"))
	if err != nil {
		return model.Partial{}, fmt.Errorf("open proc stat: %w", err)
	}
	defer f.Close()
	cpu, err := parseCPUStat(f)
	if err != nil {
		return model.Partial{}, err
	}
	return model.Partial{CPU: cpu}, nil
}

func parseCPUStat(r io.Reader) (*model.CPU, error) {
	seconds := make(map[string]map[string]float64)
	scanner := bufio.NewScanner(r)
	logical := 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		if fields[0] == "cpu" {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu")); err != nil {
			continue
		}
		if len(fields) < 5 {
			return nil, fmt.Errorf("parse proc stat: CPU row has %d fields", len(fields))
		}
		values := make(map[string]float64)
		for i, mode := range cpuModes {
			if i+1 >= len(fields) {
				break
			}
			v, err := strconv.ParseUint(fields[i+1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse proc stat %s %s: %w", fields[0], mode, err)
			}
			values[mode] = float64(v) / linuxUserHZ
		}
		seconds[fields[0]] = values
		logical++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan proc stat: %w", err)
	}
	if logical == 0 {
		return nil, fmt.Errorf("parse proc stat: no logical CPUs")
	}
	return &model.CPU{LogicalCount: logical, Seconds: seconds}, nil
}
