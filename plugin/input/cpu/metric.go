package cpu

import (
	cpuUtil "github.com/shirou/gopsutil/v3/cpu"

	"telemetry/internal"
	"telemetry/models"
)

type metric struct {
	LastStats []cpuUtil.TimesStat
	CpuInfo   []cpuUtil.InfoStat
}

func NewCPUMetric() *metric {
	return &metric{}
}

func (m *metric) IsMetric() {
}

func (m *metric) Copy() models.Metric {
	m2 := &metric{
		LastStats: make([]cpuUtil.TimesStat, len(m.LastStats)),
		CpuInfo:   make([]cpuUtil.InfoStat, len(m.CpuInfo)),
	}

	m2.CpuInfo = internal.DeepCopy(m.CpuInfo).([]cpuUtil.InfoStat)
	m2.LastStats = internal.DeepCopy(m.LastStats).([]cpuUtil.TimesStat)

	return m2
}
