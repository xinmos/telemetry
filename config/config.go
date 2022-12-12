package config

import (
	"log"
	"path"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"

	"telemetry/models"
	"telemetry/plugin/input/cisco_telemetry_mdt"
	"telemetry/plugin/input/cpu"
	"telemetry/plugin/output/file"
	"telemetry/plugin/output/kafka"
	"telemetry/plugin/serializers"
	"telemetry/plugin/serializers/json"
)

type Config struct {
	Agent   AgentConfig
	Inputs  map[string]any
	Outputs map[string]any

	RunningInputs  []*models.RunningInput
	RunningOutputs []*models.RunningOutput
}

type AgentConfig struct {
	// Interval at which to gather information
	Interval Duration `toml:"interval"`

	// RoundInterval rounds collection interval to 'interval'.
	//     ie, if Interval=10s then always collect on :00, :10, :20, etc.
	RoundInterval bool `toml:"round_interval"`

	// CollectionJitter is used to jitter the collection by a random amount.
	// Each plugin will sleep for a random time within jitter before collecting.
	// This can be used to avoid many plugins querying things like sysfs at the
	// same time, which can have a measurable effect on the system.
	CollectionJitter Duration `toml:"collection_jitter"`

	// CollectionOffset is used to shift the collection by the given amount.
	// This can be used to avoid many plugins querying constraint devices
	// at the same time by manually scheduling them in time.
	CollectionOffset Duration `toml:"collection_offset"`

	// FlushInterval is the Interval at which to flush data
	FlushInterval Duration `toml:"flush_interval"`

	// FlushJitter Jitters the flush interval by a random amount.
	// This is primarily to avoid large write spikes for users running a large
	// number of telegraf instances.
	// ie, a jitter of 5s and interval 10s means flushes will happen every 10-15s
	FlushJitter Duration `toml:"flush_jitter"`

	// MetricBatchSize is the maximum number of metrics that is written to an
	// output plugin in one call.
	MetricBatchSize int `toml:"metric_batch_size"`

	// MetricBufferLimit is the max number of metrics that each output plugin
	// will cache. The buffer is cleared when a successful write occurs. When
	// full, the oldest metrics will be overwritten. This number should be a
	// multiple of MetricBatchSize. Due to current implementation, this could
	// not be less than 2 times MetricBatchSize.
	MetricBufferLimit int `toml:"metric_buffer_limit"`

	// Debug is the option for running in debug mode
	LogLevel string `toml:"log_level"`

	// Name of the file to be logged to when using the "file" logtarget.  If set to
	// the empty string then logs are written to stderr.
	Logfile string `toml:"logfile"`

	// Interval is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	LogfileRotationInterval int `toml:"logfile_rotation_interval"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	LogfileRotationMaxSize int `toml:"logfile_rotation_max_size"`

	// MaxArchives is the maximum number of old log files to retain.  The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted.)
	LogfileRotationMaxArchives int `toml:"logfile_rotation_max_archives"`

	// Compress determines if the rotated log files should be compressed
	// using gzip. The default is not to perform compression.
	LogfileRotationMaxCompress bool `toml:"logfile_rotation_max_compress"`
}

func NewConfig(filepath string) *Config {
	var cfg *Config
	_, filename, _, _ := runtime.Caller(0)
	filePath := path.Join(path.Dir(filename), filepath)

	_, err := toml.DecodeFile(filePath, &cfg)
	if err != nil {
		log.Fatal(err)
	}

	return cfg
}

func (c *Config) addInput(input models.Input, name string, cfgs any) error {
	for _, cfg := range cfgs.([]map[string]any) {
		runInput := models.RunningInput{
			Input: input,
			Name:  name,
		}
		// init config
		err := runInput.Input.ParseConfig(cfg)
		if err != nil {
			return err
		}
		c.RunningInputs = append(c.RunningInputs, &runInput)
	}
	return nil
}

func (c *Config) addOutput(output models.Output, name string, cfgs any) error {
	serializer, _ := json.NewSerializer(1*time.Millisecond, "2006-01-02 15:04:05.000", "")
	for _, cfg := range cfgs.([]map[string]any) {
		runOuput := models.NewRunningOutput(output, name, c.Agent.MetricBatchSize, c.Agent.MetricBufferLimit)
		// init config
		err := runOuput.Output.ParseConfig(cfg)
		if err != nil {
			return err
		}

		if ro, ok := runOuput.Output.(serializers.SerializerOutput); ok {
			ro.SetSerializer(serializer)
		}
		c.RunningOutputs = append(c.RunningOutputs, runOuput)
	}
	return nil
}

func (c *Config) LoadAll() error {
	for input, inputCfg := range c.Inputs {
		switch input {
		case "cisco_telemetry_mdt":
			err := c.addInput(cisco_telemetry_mdt.NewCiscoTelemetryMDT(), input, inputCfg)
			if err != nil {
				return err
			}
		case "cpu":
			err := c.addInput(cpu.NewCPUStats(), input, inputCfg)
			if err != nil {
				return err
			}
		}
	}

	for output, outputCfg := range c.Outputs {
		switch output {
		case "file":
			err := c.addOutput(file.NewFile(), output, outputCfg)
			if err != nil {
				return err
			}
		case "kafka":
			err := c.addOutput(kafka.NewKafka(), output, outputCfg)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
