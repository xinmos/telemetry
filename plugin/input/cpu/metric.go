package cpu

import cpuUtil "github.com/shirou/gopsutil/v3/cpu"

type metric struct {
	LastStats []cpuUtil.TimesStat
	CpuInfo   []cpuUtil.InfoStat
}

func NewCPUMetric() *metric {
	return &metric{}
}

func (m *metric) AddField() {
}
