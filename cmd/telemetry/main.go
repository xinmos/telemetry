package main

import (
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

type GlobalFlags struct {
	config string
}

func runApp(m App) error {
	action := func(c *cli.Context) error {
		g := GlobalFlags{
			config: c.String("config"),
		}
		if g.config == "" {
			g.config = "telemetry.toml"
		}

		m.Init(g)
		return m.Run()
	}

	app := &cli.App{
		Name:  "telemetry",
		Usage: "get data from telemetry models driven",
		Flags: append([]cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path of config `file`",
			},
		}),
		Action: action,
		Commands: []*cli.Command{
			{
				Name:    "version",
				Aliases: []string{"v"},
				Usage:   "show the version of telemetry",
				Action: func(c *cli.Context) error {
					fmt.Println("Telemetry-0.0.1")
					return nil
				},
			},
		},
	}

	return app.Run(os.Args)
}

func main() {
	telemetry := Telemetry{}
	err := runApp(&telemetry)
	if err != nil {
		log.Fatal(err)
	}
}
