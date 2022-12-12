package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"telemetry/config"
	"telemetry/internal"
	"telemetry/models"
)

// inputUnit is a group of input plugins and the shared channel they write to.
//
// ┌───────┐
// │ Input │───┐
// └───────┘   │
// ┌───────┐   │     ______
// │ Input │───┼──▶ ()_____)
// └───────┘   │
// ┌───────┐   │
// │ Input │───┘
// └───────┘
type inputUnit struct {
	dst    chan<- models.Metric
	inputs []*models.RunningInput
}

// outputUnit is a group of Outputs and their source channel.  Metrics on the
// channel are written to all outputs.

//                            ┌────────┐
//                       ┌──▶ │ Output │
//                       │    └────────┘
//  ______     ┌─────┐   │    ┌────────┐
// ()_____)──▶ │ Fan │───┼──▶ │ Output │
//             └─────┘   │    └────────┘
//                       │    ┌────────┐
//                       └──▶ │ Output │
//                            └────────┘

type outputUnit struct {
	src     <-chan models.Metric
	outputs []*models.RunningOutput
}

type Agent struct {
	Config *config.Config

	log *logrus.Entry
}

func (a *Agent) Run(ctx context.Context) error {
	var err error
	a.log = models.NewLogger("agent")
	a.log.Debugf("starting plugins")

	err = a.initPlugins()
	if err != nil {
		return err
	}

	startTime := time.Now()

	a.log.Debugf("Connecting outputs")
	next, outUnit, err := a.startOutputs(ctx, a.Config.RunningOutputs)
	if err != nil {
		return err
	}

	a.log.Debugf("Starting inputs")
	inUnit, err := a.startInputs(next, a.Config.RunningInputs)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.runOutputs(outUnit)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.runInputs(ctx, startTime, inUnit)
	}()

	wg.Wait()

	return err
}

func (a *Agent) runInputs(ctx context.Context, startTime time.Time, unit *inputUnit) {
	a.log.Debugf("run inputs: %s", a.getPlugins(a.Config.Inputs))

	var wg sync.WaitGroup
	tickers := make([]Ticker, 0, len(unit.inputs))

	for _, input := range unit.inputs {
		interval := time.Duration(a.Config.Agent.Interval)
		jitter := time.Duration(a.Config.Agent.CollectionJitter)
		offset := time.Duration(a.Config.Agent.CollectionOffset)

		var ticker Ticker
		if a.Config.Agent.RoundInterval {
			ticker = NewAlignedTicker(startTime, interval, jitter, offset)
		} else {
			ticker = NewUnalignedTicker(interval, jitter, offset)
		}
		tickers = append(tickers, ticker)

		acc := NewAccumulator(unit.dst)

		wg.Add(1)
		go func(input *models.RunningInput) {
			defer wg.Done()
			a.gatherLoop(ctx, acc, input, ticker, interval)
		}(input)
	}
	defer stopTickers(tickers)
	wg.Wait()

	a.log.Debugf("Stopping service inputs")
	stopServiceInputs(unit.inputs)

	close(unit.dst)
	a.log.Debugf("Input channel closed")
}

func (a *Agent) getPlugins(plugins map[string]any) []string {
	j := 0
	keys := make([]string, len(plugins))
	for k := range plugins {
		keys[j] = k
		j++
	}
	return keys
}

func (a *Agent) gatherLoop(ctx context.Context, acc models.Accumulator, input *models.RunningInput, ticker Ticker, interval time.Duration) {
	defer panicRecover(input)

	for {
		select {
		case <-ticker.Elapsed():
			err := a.gatherOnce(acc, input, ticker, interval)
			if err != nil {
				acc.AddError(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func stopServiceInputs(inputs []*models.RunningInput) {
	for _, input := range inputs {
		if si, ok := input.Input.(models.ServiceInput); ok {
			si.Stop()
		}
	}
}

func panicRecover(input *models.RunningInput) {
	if err := recover(); err != nil {
		trace := make([]byte, 2048)
		runtime.Stack(trace, true)
		log.Printf("E! FATAL: [%s] panicked: %s, Stack:\n%s", input.Name, err, trace)
	}
}

func (a *Agent) gatherOnce(acc models.Accumulator, input *models.RunningInput, ticker Ticker, interval time.Duration) error {
	done := make(chan error)
	go func() {
		done <- input.Gather(acc)
	}()

	// Only warn after interval seconds, even if the interval is started late.
	// Intervals can start late if the previous interval went over or due to
	// clock changes.
	slowWarning := time.NewTicker(interval)
	defer slowWarning.Stop()

	for {
		select {
		case err := <-done:
			return err
		case <-slowWarning.C:
			log.Printf("W! [%s] Collection took longer than expected; not complete after interval of %s",
				input.Name, interval)
		case <-ticker.Elapsed():
			log.Printf("D! [%s] Previous collection has not completed; scheduled collection skipped",
				input.Name)
		}
	}
}

func (a *Agent) initPlugins() error {
	a.log.Debugf("init inputs: %v", a.getPlugins(a.Config.Inputs))
	for _, input := range a.Config.RunningInputs {
		err := input.Init()
		if err != nil {
			return fmt.Errorf("could not initialize input %s: %v", input.Name, err)
		}
	}

	a.log.Debugf("init outputs: %v", a.getPlugins(a.Config.Outputs))
	for _, output := range a.Config.RunningOutputs {
		err := output.Init()
		if err != nil {
			return fmt.Errorf("could not initialize output %s: %v", output.Name, err)
		}
	}
	return nil
}

func (a *Agent) startInputs(dst chan<- models.Metric, inputs []*models.RunningInput) (*inputUnit, error) {
	a.log.Debugf("Starting service inputs")

	unit := &inputUnit{
		dst: dst,
	}

	for _, input := range inputs {
		if si, ok := input.Input.(models.ServiceInput); ok {
			acc := NewAccumulator(dst)
			err := si.Start(acc)
			if err != nil {
				stopServiceInputs(unit.inputs)
				return nil, fmt.Errorf("starting input %s: %w", input.Name, err)
			}
		}
		unit.inputs = append(unit.inputs, input)
	}

	return unit, nil
}

func (a *Agent) startOutputs(ctx context.Context, outputs []*models.RunningOutput) (chan<- models.Metric, *outputUnit, error) {
	src := make(chan models.Metric, 100)
	unit := &outputUnit{src: src}
	for _, output := range outputs {
		err := a.connectOutput(ctx, output)
		if err != nil {
			for _, output := range unit.outputs {
				output.Close()
			}
			return nil, nil, fmt.Errorf("connecting output %s: %w", output.Name, err)
		}

		unit.outputs = append(unit.outputs, output)
	}

	return src, unit, nil
}

func (a *Agent) connectOutput(ctx context.Context, output *models.RunningOutput) error {
	a.log.Debugf("Attempting connection to [%s]", output.Name)
	err := output.Output.Connect()
	if err != nil {
		a.log.Errorf("Failed to connect to [%s], retrying in 15s, "+
			"error was '%s'", output.Name, err)

		err := internal.SleepContext(ctx, 15*time.Second)
		if err != nil {
			return err
		}

		err = output.Output.Connect()
		if err != nil {
			return fmt.Errorf("error connecting to output %q: %w", output.Name, err)
		}
	}
	a.log.Debugf("Successfully connected to %s", output.Name)
	return nil
}

func (a *Agent) runOutputs(unit *outputUnit) {
	var wg sync.WaitGroup

	// Start flush loop
	interval := time.Duration(a.Config.Agent.FlushInterval)
	jitter := time.Duration(a.Config.Agent.FlushJitter)

	ctx, cancel := context.WithCancel(context.Background())

	for _, output := range unit.outputs {
		wg.Add(1)
		go func(output *models.RunningOutput) {
			defer wg.Done()

			ticker := NewRollingTicker(interval, jitter)
			defer ticker.Stop()

			a.flushLoop(ctx, output, ticker)
		}(output)
	}

	for metric := range unit.src {
		for i, output := range unit.outputs {
			if i == len(a.Config.RunningOutputs)-1 {
				output.AddMetric(metric)
			} else {
				output.AddMetric(metric.Copy())
			}
		}
	}

	a.log.Infoln("Hang on, flushing any cached metrics before shutdown")
	cancel()
	wg.Wait()

	a.log.Infoln("Stopping running outputs")
	stopRunningOutputs(unit.outputs)
}

func (a *Agent) flushLoop(ctx context.Context, output *models.RunningOutput, ticker *RollingTicker) {
	logError := func(err error) {
		if err != nil {
			a.log.Errorf("Error writing to %s: %v", output.Name, err)
		}
	}

	// watch for flush requests
	flushRequested := make(chan os.Signal, 1)

	for {
		// Favor shutdown over other methods.
		select {
		case <-ctx.Done():
			logError(a.flushOnce(output, ticker, output.Write))
			return
		default:
		}

		select {
		case <-ctx.Done():
			logError(a.flushOnce(output, ticker, output.Write))
			return
		case <-ticker.Elapsed():
			logError(a.flushOnce(output, ticker, output.Write))
		case <-flushRequested:
			logError(a.flushOnce(output, ticker, output.Write))
		case <-output.BatchReady:
			logError(a.flushBatch(output, output.WriteBatch))
		}
	}
}

func (a *Agent) flushOnce(output *models.RunningOutput, ticker *RollingTicker, writeFunc func() error) error {
	done := make(chan error)
	go func() {
		done <- writeFunc()
	}()

	for {
		select {
		case err := <-done:
			output.LogBufferStatus()
			return err
		case <-ticker.Elapsed():
			a.log.Warnf("[%q] did not complete within its flush interval", output.Name)
			output.LogBufferStatus()
		}
	}
}

func (a *Agent) flushBatch(output *models.RunningOutput, writeFunc func() error) error {
	err := writeFunc()
	output.LogBufferStatus()
	return err
}

func stopRunningOutputs(outputs []*models.RunningOutput) {
	for _, output := range outputs {
		output.Close()
	}
}

func stopTickers(tickers []Ticker) {
	for _, ticker := range tickers {
		ticker.Stop()
	}
}

func NewAgent(cfg *config.Config) *Agent {
	return &Agent{
		Config: cfg,
	}
}
