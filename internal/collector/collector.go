package collector

import (
	"context"
	"regexp"

	"sma/internal/config"
	"sma/internal/model"
)

type Collector interface {
	Name() string
	Collect(context.Context) (model.Partial, error)
}

func Enabled(cfg config.Config) []Collector {
	var collectors []Collector
	if cfg.EnabledCollectors["cpu"] {
		collectors = append(collectors, CPUCollector{ProcPath: cfg.ProcPath})
	}
	if cfg.EnabledCollectors["memory"] {
		collectors = append(collectors, MemoryCollector{ProcPath: cfg.ProcPath})
	}
	if cfg.EnabledCollectors["filesystem"] {
		collectors = append(collectors, FilesystemCollector{
			ProcPath: cfg.ProcPath, ExcludeFSTypes: cfg.FilesystemExcludeFSTypes,
			ExcludeMountpoints: cfg.FilesystemExcludeMounts,
		})
	}
	if cfg.EnabledCollectors["disk"] {
		collectors = append(collectors, DiskCollector{ProcPath: cfg.ProcPath, ExcludeDevices: cfg.DiskExcludeDevices})
	}
	if cfg.EnabledCollectors["load"] {
		collectors = append(collectors, LoadCollector{ProcPath: cfg.ProcPath})
	}
	if cfg.EnabledCollectors["uptime"] {
		collectors = append(collectors, UptimeCollector{ProcPath: cfg.ProcPath})
	}
	return collectors
}

func match(re *regexp.Regexp, value string) bool {
	return re != nil && re.MatchString(value)
}
