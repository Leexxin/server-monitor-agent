package metric

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sma/internal/buildinfo"
	"sma/internal/model"
)

type AgentStats struct {
	Scrapes uint64
	Errors  uint64
}

type encoder struct{ bytes.Buffer }

func Prometheus(snapshot model.Snapshot, build buildinfo.Info, stats AgentStats) []byte {
	e := &encoder{}
	e.family("sma_build_info", "Build information for the server monitor agent.", "gauge")
	e.sample("sma_build_info", map[string]string{"version": build.Version, "revision": build.Revision, "go_version": build.GoVersion}, 1)
	e.family("sma_scrapes_total", "Total number of Prometheus scrapes.", "counter")
	e.sample("sma_scrapes_total", nil, float64(stats.Scrapes))
	e.family("sma_scrape_errors_total", "Total number of scrapes with collector errors.", "counter")
	e.sample("sma_scrape_errors_total", nil, float64(stats.Errors))

	if snapshot.CPU != nil {
		e.family("sma_cpu_seconds_total", "Seconds CPUs spent in each mode.", "counter")
		cpus := sortedKeys(snapshot.CPU.Seconds)
		for _, cpu := range cpus {
			modes := sortedKeys(snapshot.CPU.Seconds[cpu])
			for _, mode := range modes {
				e.sample("sma_cpu_seconds_total", map[string]string{"cpu": cpu, "mode": mode}, snapshot.CPU.Seconds[cpu][mode])
			}
		}
		e.family("sma_cpu_logical_count", "Number of logical CPUs.", "gauge")
		e.sample("sma_cpu_logical_count", nil, float64(snapshot.CPU.LogicalCount))
	}
	if m := snapshot.Memory; m != nil {
		memory := []struct {
			name, help string
			value      uint64
		}{
			{"sma_memory_total_bytes", "Total physical memory in bytes.", m.TotalBytes},
			{"sma_memory_available_bytes", "Memory available for starting new applications in bytes.", m.AvailableBytes},
			{"sma_memory_free_bytes", "Unused physical memory in bytes.", m.FreeBytes},
			{"sma_memory_cached_bytes", "Cached memory in bytes.", m.CachedBytes},
			{"sma_memory_buffers_bytes", "Filesystem buffer memory in bytes.", m.BuffersBytes},
			{"sma_memory_swap_total_bytes", "Total swap memory in bytes.", m.SwapTotalBytes},
			{"sma_memory_swap_free_bytes", "Unused swap memory in bytes.", m.SwapFreeBytes},
		}
		for _, item := range memory {
			e.family(item.name, item.help, "gauge")
			e.sample(item.name, nil, float64(item.value))
		}
	}
	if snapshot.Load != nil {
		for _, item := range []struct {
			name, help string
			value      float64
		}{
			{"sma_load1", "One minute load average.", snapshot.Load.Load1},
			{"sma_load5", "Five minute load average.", snapshot.Load.Load5},
			{"sma_load15", "Fifteen minute load average.", snapshot.Load.Load15},
		} {
			e.family(item.name, item.help, "gauge")
			e.sample(item.name, nil, item.value)
		}
	}
	if snapshot.UptimeSeconds != nil {
		e.family("sma_uptime_seconds", "System uptime in seconds.", "gauge")
		e.sample("sma_uptime_seconds", nil, *snapshot.UptimeSeconds)
	}
	if snapshot.BootTime != nil {
		e.family("sma_boot_time_seconds", "Unix time at which the system booted.", "gauge")
		e.sample("sma_boot_time_seconds", nil, *snapshot.BootTime)
	}
	encodeFilesystems(e, snapshot.Filesystems)
	encodeDisks(e, snapshot.Disks)

	e.family("sma_collector_duration_seconds", "Duration of the latest collection by collector.", "gauge")
	e.family("sma_collector_success", "Whether the latest collection succeeded.", "gauge")
	for _, name := range sortedKeys(snapshot.CollectorInfo) {
		status := snapshot.CollectorInfo[name]
		e.sample("sma_collector_duration_seconds", map[string]string{"collector": name}, status.Duration.Seconds())
		value := 0.0
		if status.Success {
			value = 1
		}
		e.sample("sma_collector_success", map[string]string{"collector": name}, value)
	}
	return e.Bytes()
}

func encodeFilesystems(e *encoder, filesystems []model.Filesystem) {
	type def struct {
		name, help string
		value      func(model.Filesystem) float64
	}
	defs := []def{
		{"sma_filesystem_size_bytes", "Filesystem size in bytes.", func(v model.Filesystem) float64 { return float64(v.SizeBytes) }},
		{"sma_filesystem_available_bytes", "Filesystem space available to unprivileged users in bytes.", func(v model.Filesystem) float64 { return float64(v.AvailableBytes) }},
		{"sma_filesystem_free_bytes", "Filesystem free space in bytes.", func(v model.Filesystem) float64 { return float64(v.FreeBytes) }},
		{"sma_filesystem_files", "Total filesystem file nodes.", func(v model.Filesystem) float64 { return float64(v.Files) }},
		{"sma_filesystem_files_free", "Free filesystem file nodes.", func(v model.Filesystem) float64 { return float64(v.FilesFree) }},
		{"sma_filesystem_readonly", "Whether the filesystem is read-only.", func(v model.Filesystem) float64 {
			if v.ReadOnly {
				return 1
			}
			return 0
		}},
	}
	for _, d := range defs {
		e.family(d.name, d.help, "gauge")
		for _, fs := range filesystems {
			e.sample(d.name, map[string]string{"device": fs.Device, "mountpoint": fs.Mountpoint, "fstype": fs.FilesystemType}, d.value(fs))
		}
	}
}

func encodeDisks(e *encoder, disks []model.Disk) {
	type def struct {
		name, help, kind string
		value            func(model.Disk) float64
	}
	defs := []def{
		{"sma_disk_reads_completed_total", "Completed disk reads.", "counter", func(v model.Disk) float64 { return float64(v.ReadsCompleted) }},
		{"sma_disk_read_bytes_total", "Bytes read from disk.", "counter", func(v model.Disk) float64 { return float64(v.ReadBytes) }},
		{"sma_disk_read_seconds_total", "Seconds spent reading from disk.", "counter", func(v model.Disk) float64 { return v.ReadSeconds }},
		{"sma_disk_writes_completed_total", "Completed disk writes.", "counter", func(v model.Disk) float64 { return float64(v.WritesCompleted) }},
		{"sma_disk_written_bytes_total", "Bytes written to disk.", "counter", func(v model.Disk) float64 { return float64(v.WrittenBytes) }},
		{"sma_disk_write_seconds_total", "Seconds spent writing to disk.", "counter", func(v model.Disk) float64 { return v.WriteSeconds }},
		{"sma_disk_io_now", "Disk I/O operations currently in progress.", "gauge", func(v model.Disk) float64 { return float64(v.IOInProgress) }},
		{"sma_disk_io_seconds_total", "Seconds during which disk I/O was in progress.", "counter", func(v model.Disk) float64 { return v.IOSeconds }},
		{"sma_disk_io_weighted_seconds_total", "Weighted seconds spent doing disk I/O.", "counter", func(v model.Disk) float64 { return v.WeightedIOSeconds }},
	}
	for _, d := range defs {
		e.family(d.name, d.help, d.kind)
		for _, disk := range disks {
			e.sample(d.name, map[string]string{"device": disk.Device}, d.value(disk))
		}
	}
}

func (e *encoder) family(name, help, kind string) {
	fmt.Fprintf(&e.Buffer, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

func (e *encoder) sample(name string, labels map[string]string, value float64) {
	e.WriteString(name)
	if len(labels) > 0 {
		e.WriteByte('{')
		keys := sortedKeys(labels)
		for i, key := range keys {
			if i > 0 {
				e.WriteByte(',')
			}
			fmt.Fprintf(&e.Buffer, `%s="%s"`, key, escapeLabel(labels[key]))
		}
		e.WriteByte('}')
	}
	e.WriteByte(' ')
	e.WriteString(strconv.FormatFloat(value, 'g', -1, 64))
	e.WriteByte('\n')
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
