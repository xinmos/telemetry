package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/coreos/go-systemd/daemon"
	"telemetry/agent"
	"telemetry/config"
	"telemetry/models"
)

var stop chan struct{}

type App interface {
	Init(GlobalFlags)
	Run() error
}

type Telemetry struct {
	inputFilters  []string
	outputFilters []string

	GlobalFlags
}

func (t *Telemetry) Init(g GlobalFlags) {
	t.GlobalFlags = g
}

func (t *Telemetry) Run() error {
	stop = make(chan struct{})
	return t.reloadLoop()
}

func (t *Telemetry) reloadLoop() error {
	reload := make(chan bool, 1)
	reload <- true
	for <-reload {
		reload <- false
		ctx, cancel := context.WithCancel(context.Background())

		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)

		go func() {
			select {
			case sig := <-signals:
				if sig == syscall.SIGHUP {
					log.Printf("Info: Reload Telemetry config")
					<-reload
					reload <- true
				}
				cancel()
			case <-stop:
				cancel()
			}
		}()

		err := t.runAgent(ctx)
		if err != nil && err != context.Canceled {
			return fmt.Errorf("[telemetry] Error running agent: %v", err)
		}
	}

	return nil
}

func (t *Telemetry) runAgent(ctx context.Context) error {
	cfg := config.NewConfig(t.config)
	if err := cfg.LoadAll(); err != nil {
		return err
	}

	models.InitLogger(
		cfg.Agent.Logfile,
		cfg.Agent.LogfileRotationMaxSize,
		cfg.Agent.LogfileRotationMaxArchives,
		cfg.Agent.LogfileRotationInterval,
		cfg.Agent.LogLevel,
		cfg.Agent.LogfileRotationMaxCompress)
	log.Printf("starting Telemetry")

	// Notify systemd that telegraf is ready
	// SdNotify() only tries to notify if the NOTIFY_SOCKET environment is set, so it's safe to call when systemd isn't present.
	// Ignore the return values here because they're not valid for platforms that don't use systemd.
	// For platforms that use systemd, telegraf doesn't log if the notification failed.
	_, _ = daemon.SdNotify(false, daemon.SdNotifyReady)

	ag := agent.NewAgent(cfg)

	return ag.Run(ctx)
}
