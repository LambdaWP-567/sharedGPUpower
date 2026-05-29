package resources

import (
	"runtime"
	"time"
)

type Stats struct {
	CPUPercent float32
	MemUsedGB  float32
	MemTotalGB float32
	Timestamp  time.Time
}

// Monitor periodically samples resource usage.
type Monitor struct {
	interval time.Duration
	ch       chan Stats
}

func NewMonitor(interval time.Duration) *Monitor {
	return &Monitor{
		interval: interval,
		ch:       make(chan Stats, 4),
	}
}

func (m *Monitor) Start() {
	go func() {
		t := time.NewTicker(m.interval)
		defer t.Stop()
		for range t.C {
			m.ch <- m.sample()
		}
	}()
}

func (m *Monitor) Stats() <-chan Stats {
	return m.ch
}

func (m *Monitor) sample() Stats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return Stats{
		MemUsedGB:  float32(memStats.Alloc) / 1e9,
		MemTotalGB: float32(memStats.Sys) / 1e9,
		Timestamp:  time.Now(),
	}
}

// HardwareInfo collects static hardware info.
type HardwareInfo struct {
	CPUCores int
	Arch     string
	OS       string
}

func GetHardwareInfo() HardwareInfo {
	return HardwareInfo{
		CPUCores: runtime.NumCPU(),
		Arch:     runtime.GOARCH,
		OS:       runtime.GOOS,
	}
}
