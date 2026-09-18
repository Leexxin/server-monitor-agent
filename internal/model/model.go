package model

import "time"

const SchemaVersion = "v1"

type Snapshot struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Timestamp     time.Time                  `json:"timestamp"`
	Host          Host                       `json:"host"`
	CPU           *CPU                       `json:"cpu,omitempty"`
	Memory        *Memory                    `json:"memory,omitempty"`
	Filesystems   []Filesystem               `json:"filesystems"`
	Disks         []Disk                     `json:"disks"`
	Load          *Load                      `json:"load,omitempty"`
	UptimeSeconds *float64                   `json:"uptimeSeconds,omitempty"`
	BootTime      *float64                   `json:"bootTimeSeconds,omitempty"`
	Errors        []CollectorError           `json:"errors"`
	CollectorInfo map[string]CollectorStatus `json:"-"`
}

type Host struct {
	Hostname string `json:"hostname"`
}

type CPU struct {
	LogicalCount int                           `json:"logicalCount"`
	Seconds      map[string]map[string]float64 `json:"seconds"`
}

type Memory struct {
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	FreeBytes      uint64 `json:"freeBytes"`
	CachedBytes    uint64 `json:"cachedBytes"`
	BuffersBytes   uint64 `json:"buffersBytes"`
	SwapTotalBytes uint64 `json:"swapTotalBytes"`
	SwapFreeBytes  uint64 `json:"swapFreeBytes"`
}

type Filesystem struct {
	Device         string `json:"device"`
	Mountpoint     string `json:"mountpoint"`
	FilesystemType string `json:"filesystemType"`
	SizeBytes      uint64 `json:"sizeBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	FreeBytes      uint64 `json:"freeBytes"`
	Files          uint64 `json:"files"`
	FilesFree      uint64 `json:"filesFree"`
	ReadOnly       bool   `json:"readOnly"`
}

type Disk struct {
	Device            string  `json:"device"`
	ReadsCompleted    uint64  `json:"readsCompleted"`
	ReadBytes         uint64  `json:"readBytes"`
	ReadSeconds       float64 `json:"readSeconds"`
	WritesCompleted   uint64  `json:"writesCompleted"`
	WrittenBytes      uint64  `json:"writtenBytes"`
	WriteSeconds      float64 `json:"writeSeconds"`
	IOInProgress      uint64  `json:"ioInProgress"`
	IOSeconds         float64 `json:"ioSeconds"`
	WeightedIOSeconds float64 `json:"weightedIOSeconds"`
}

type Load struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type CollectorError struct {
	Collector string `json:"collector"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type CollectorStatus struct {
	Success  bool
	Duration time.Duration
}

type Partial struct {
	CPU         *CPU
	Memory      *Memory
	Filesystems []Filesystem
	Disks       []Disk
	Load        *Load
	Uptime      *float64
	BootTime    *float64
}

func (s *Snapshot) Merge(p Partial) {
	if p.CPU != nil {
		s.CPU = p.CPU
	}
	if p.Memory != nil {
		s.Memory = p.Memory
	}
	if p.Filesystems != nil {
		s.Filesystems = p.Filesystems
	}
	if p.Disks != nil {
		s.Disks = p.Disks
	}
	if p.Load != nil {
		s.Load = p.Load
	}
	if p.Uptime != nil {
		s.UptimeSeconds = p.Uptime
	}
	if p.BootTime != nil {
		s.BootTime = p.BootTime
	}
}
