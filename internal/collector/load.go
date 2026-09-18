package collector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sma/internal/model"
)

type LoadCollector struct{ ProcPath string }

func (LoadCollector) Name() string { return "load" }

func (c LoadCollector) Collect(_ context.Context) (model.Partial, error) {
	data, err := os.ReadFile(filepath.Join(c.ProcPath, "loadavg"))
	if err != nil {
		return model.Partial{}, fmt.Errorf("read loadavg: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return model.Partial{}, fmt.Errorf("parse loadavg: expected at least 3 fields")
	}
	values := [3]float64{}
	for i := range values {
		values[i], err = strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return model.Partial{}, fmt.Errorf("parse loadavg field %d: %w", i, err)
		}
	}
	return model.Partial{Load: &model.Load{Load1: values[0], Load5: values[1], Load15: values[2]}}, nil
}
