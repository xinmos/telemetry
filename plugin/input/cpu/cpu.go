package cpu

import (
	_ "embed"
	"encoding/json"
	"fmt"

	cpuUtil "github.com/shirou/gopsutil/v3/cpu"
	"github.com/sirupsen/logrus"

	"telemetry/models"
)

type CPUStats struct {
	PerCPU   bool `json:"percpu"`
	TotalCPU bool `json:"totalcpu"`

	log *logrus.Entry
}

func (c *CPUStats) AddField() {
}

func NewCPUStats() *CPUStats {
	return &CPUStats{
		log: models.NewLogger("inputs.cpu"),
	}
}

func (c *CPUStats) Gather(acc models.Accumulator) error {
	var err error
	m := NewCPUMetric()
	m.LastStats, err = cpuUtil.Times(c.PerCPU)
	if err != nil {
		return err
	}
	m.CpuInfo, err = cpuUtil.Info()
	if err != nil {
		return err
	}

	acc.AddMetric(m)
	return nil
}

func (c *CPUStats) Init() error {
	return nil
}

func (c *CPUStats) ParseConfig(cfg map[string]any) error {
	tmp, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	err = json.Unmarshal(tmp, c)
	if err != nil {
		return fmt.Errorf("[cpu] config error: %v", err)
	}
	return nil
}
