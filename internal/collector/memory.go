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

type MemoryCollector struct{ ProcPath string }

func (MemoryCollector) Name() string { return "memory" }

func (c MemoryCollector) Collect(_ context.Context) (model.Partial, error) {
	f, err := os.Open(filepath.Join(c.ProcPath, "meminfo"))
	if err != nil {
		return model.Partial{}, fmt.Errorf("open meminfo: %w", err)
	}
	defer f.Close()
	memory, err := parseMeminfo(f)
	if err != nil {
		return model.Partial{}, err
	}
	return model.Partial{Memory: memory}, nil
}

func parseMeminfo(r io.Reader) (*model.Memory, error) {
	values := make(map[string]uint64)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse meminfo %s: %w", name, err)
		}
		if len(fields) >= 3 && fields[2] == "kB" {
			value *= 1024
		}
		values[name] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan meminfo: %w", err)
	}
	if values["MemTotal"] == 0 {
		return nil, fmt.Errorf("parse meminfo: MemTotal is missing or zero")
	}
	cached := saturatingSub(values["Cached"]+values["SReclaimable"], values["Shmem"])
	available, ok := values["MemAvailable"]
	if !ok {
		available = values["MemFree"] + values["Buffers"] + cached
	}
	return &model.Memory{
		TotalBytes: values["MemTotal"], AvailableBytes: available, FreeBytes: values["MemFree"],
		CachedBytes: cached, BuffersBytes: values["Buffers"], SwapTotalBytes: values["SwapTotal"],
		SwapFreeBytes: values["SwapFree"],
	}, nil
}

func saturatingSub(a, b uint64) uint64 {
	if b > a {
		return 0
	}
	return a - b
}
