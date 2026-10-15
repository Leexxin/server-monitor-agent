package collector

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"sma/internal/model"
)

var (
	kernel             = windows.NewLazySystemDLL("kernel32.dll")
	getSystemTimes     = kernel.NewProc("GetSystemTimes")
	globalMemoryStatus = kernel.NewProc("GlobalMemoryStatusEx")
	getTickCount       = kernel.NewProc("GetTickCount64")
	getProcessorCount  = kernel.NewProc("GetActiveProcessorCount")
)

type CPUCollector struct{ ProcPath string }

func (CPUCollector) Name() string { return "cpu" }
func (CPUCollector) Collect(ctx context.Context) (model.Partial, error) {
	if err := ctx.Err(); err != nil {
		return model.Partial{}, err
	}
	count, _, _ := getProcessorCount.Call(0xffff) // ALL_PROCESSOR_GROUPS
	if count > 64 {
		return model.Partial{}, errors.New("Windows CPU collection currently supports at most 64 logical processors")
	}
	var idle, kernelTime, user windows.Filetime
	ok, _, err := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernelTime)), uintptr(unsafe.Pointer(&user)))
	if ok == 0 {
		return model.Partial{}, fmt.Errorf("GetSystemTimes: %w", err)
	}
	seconds := func(v windows.Filetime) float64 {
		return float64(uint64(v.HighDateTime)<<32|uint64(v.LowDateTime)) / 1e7
	}
	if count == 0 {
		count = uintptr(runtime.NumCPU())
	}
	return model.Partial{CPU: &model.CPU{LogicalCount: int(count), Seconds: map[string]map[string]float64{
		"total": {"idle": seconds(idle), "system": seconds(kernelTime) - seconds(idle), "user": seconds(user)},
	}}}, nil
}

type MemoryCollector struct{ ProcPath string }

func (MemoryCollector) Name() string { return "memory" }

type memoryStatus struct {
	Length, Load                                                                                         uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile, TotalVirtual, AvailVirtual, AvailExtendedVirtual uint64
}

func (MemoryCollector) Collect(ctx context.Context) (model.Partial, error) {
	if err := ctx.Err(); err != nil {
		return model.Partial{}, err
	}
	var m memoryStatus
	m.Length = uint32(unsafe.Sizeof(m))
	ok, _, err := globalMemoryStatus.Call(uintptr(unsafe.Pointer(&m)))
	if ok == 0 {
		return model.Partial{}, fmt.Errorf("GlobalMemoryStatusEx: %w", err)
	}
	return model.Partial{Memory: &model.Memory{TotalBytes: m.TotalPhys, AvailableBytes: m.AvailPhys}}, nil
}

type FilesystemCollector struct {
	ProcPath           string
	ExcludeFSTypes     map[string]bool
	ExcludeMountpoints *regexp.Regexp
}

func (FilesystemCollector) Name() string { return "filesystem" }

func fixedDrives() ([]string, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, err
	}
	var drives []string
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+i)
		p, _ := windows.UTF16PtrFromString(root)
		if windows.GetDriveType(p) == windows.DRIVE_FIXED {
			drives = append(drives, root)
		}
	}
	return drives, nil
}

func (c FilesystemCollector) Collect(ctx context.Context) (model.Partial, error) {
	drives, err := fixedDrives()
	if err != nil {
		return model.Partial{}, err
	}
	filesystems := []model.Filesystem{}
	var failures []error
	for _, root := range drives {
		if err := ctx.Err(); err != nil {
			return model.Partial{Filesystems: filesystems}, err
		}
		if match(c.ExcludeMountpoints, root) {
			continue
		}
		p, _ := windows.UTF16PtrFromString(root)
		var flags uint32
		var fsname [64]uint16
		if err := windows.GetVolumeInformation(p, nil, 0, nil, nil, &flags, &fsname[0], uint32(len(fsname))); err != nil {
			failures = append(failures, fmt.Errorf("volume %s: %w", root, err))
			continue
		}
		fsType := windows.UTF16ToString(fsname[:])
		if c.ExcludeFSTypes[fsType] || c.ExcludeFSTypes[strings.ToLower(fsType)] {
			continue
		}
		var available, total, free uint64
		if err := windows.GetDiskFreeSpaceEx(p, &available, &total, &free); err != nil {
			failures = append(failures, fmt.Errorf("space %s: %w", root, err))
			continue
		}
		filesystems = append(filesystems, model.Filesystem{Device: root[:2], Mountpoint: root, FilesystemType: fsType,
			SizeBytes: total, AvailableBytes: available, FreeBytes: free, ReadOnly: flags&0x80000 != 0})
	}
	return model.Partial{Filesystems: filesystems}, errors.Join(failures...)
}

type DiskCollector struct {
	ProcPath       string
	ExcludeDevices *regexp.Regexp
}

func (DiskCollector) Name() string { return "disk" }

// DISK_PERFORMANCE, from winioctl.h. Time values are 100ns intervals.
type diskPerformance struct {
	BytesRead, BytesWritten, ReadTime, WriteTime, IdleTime int64
	ReadCount, WriteCount, QueueDepth, SplitCount          uint32
	QueryTime                                              int64
	StorageDeviceNumber                                    uint32
	StorageManagerName                                     [8]uint16
}

func (c DiskCollector) Collect(ctx context.Context) (model.Partial, error) {
	drives, err := fixedDrives()
	if err != nil {
		return model.Partial{}, err
	}
	disks := []model.Disk{}
	var failures []error
	for _, root := range drives {
		if err := ctx.Err(); err != nil {
			return model.Partial{Disks: disks}, err
		}
		device := root[:2]
		if match(c.ExcludeDevices, device) {
			continue
		}
		path, _ := windows.UTF16PtrFromString(`\\.\` + device)
		h, err := windows.CreateFile(path, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			failures = append(failures, fmt.Errorf("open volume %s: %w", device, err))
			continue
		}
		var data diskPerformance
		var returned uint32
		err = windows.DeviceIoControl(h, 0x00070020, nil, 0, (*byte)(unsafe.Pointer(&data)), uint32(unsafe.Sizeof(data)), &returned, nil)
		windows.CloseHandle(h)
		if err != nil {
			failures = append(failures, fmt.Errorf("disk counters %s: %w", device, err))
			continue
		}
		if returned < uint32(unsafe.Offsetof(data.StorageManagerName))+16 {
			failures = append(failures, fmt.Errorf("short disk performance response for %s", device))
			continue
		}
		disks = append(disks, model.Disk{Device: device, ReadBytes: uint64(data.BytesRead), WrittenBytes: uint64(data.BytesWritten),
			ReadsCompleted: uint64(data.ReadCount), WritesCompleted: uint64(data.WriteCount), IOInProgress: uint64(data.QueueDepth),
			ReadSeconds: float64(data.ReadTime) / 1e7, WriteSeconds: float64(data.WriteTime) / 1e7})
	}
	return model.Partial{Disks: disks}, errors.Join(failures...)
}

type UptimeCollector struct{ ProcPath string }

func (UptimeCollector) Name() string { return "uptime" }
func (UptimeCollector) Collect(ctx context.Context) (model.Partial, error) {
	if err := ctx.Err(); err != nil {
		return model.Partial{}, err
	}
	ms, _, _ := getTickCount.Call()
	uptime := float64(ms) / 1000
	boot := float64(time.Now().UnixMilli())/1000 - uptime
	return model.Partial{Uptime: &uptime, BootTime: &boot}, nil
}

type LoadCollector struct{ ProcPath string }

func (LoadCollector) Name() string { return "load" }
func (LoadCollector) Collect(context.Context) (model.Partial, error) {
	return model.Partial{}, errors.New("load average is unsupported on Windows")
}
