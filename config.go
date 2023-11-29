// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: LicenseRef-COMCAST

package main

import (
	"fmt"
	"os"

	"github.com/goschtalt/goschtalt"
	"github.com/xmidt-org/arrange/arrangehttp"
	"github.com/xmidt-org/arrange/arrangepprof"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"
	"gopkg.in/dealancer/validate.v2"
)

type CLI struct {
	Dev   bool     `optional:"" short:"d" help:"Run in development mode."`
	Show  bool     `optional:"" short:"s" help:"Show the configuration and exit."`
	Graph string   `optional:"" short:"g" help:"Output the dependency graph to the specified file."`
	Files []string `optional:"" short:"f" help:"Specific configuration files or directories."`
}

// Config is the top level configuration for the notus service.  Everything
// is contained in this structure or it will intentially cause a failure.
type Config struct {
	Logging    sallust.Config
	Tracing    candlelight.Config
	Prometheus touchstone.Config
	Servers    Servers
}

type Servers struct {
	Health    HealthServer
	Metrics   MetricsServer
	Pprof     PprofServer
	Primary   PrimaryServer
	Alternate PrimaryServer
}

type HealthServer struct {
	HTTP arrangehttp.ServerConfig
	Path HealthPath `validate:"empty=false"`
}

type HealthPath string

type MetricsServer struct {
	HTTP arrangehttp.ServerConfig
	Path MetricsPath `validate:"empty=false"`
}

type MetricsPath string

type PrimaryServer struct {
	HTTP arrangehttp.ServerConfig
}

type PprofServer struct {
	HTTP arrangehttp.ServerConfig
	Path PprofPathPrefix
}

type PprofPathPrefix string

// Collect and process the configuration files and env vars and
// produce a configuration object.
func provideConfig(cli *CLI) (*goschtalt.Config, error) {
	gs, err := goschtalt.New(
		goschtalt.StdCfgLayout(applicationName, cli.Files...),
		goschtalt.ConfigIs("two_words"),
		goschtalt.DefaultUnmarshalOptions(
			goschtalt.WithValidator(
				goschtalt.ValidatorFunc(validate.Validate),
			),
		),

		// Seed the program with the default, built-in configuration.
		// Mark this as a default so it is ordered correctly.
		goschtalt.AddValue("built-in", goschtalt.Root, defaultConfig,
			goschtalt.AsDefault()),
	)
	if err != nil {
		return nil, err
	}

	if cli.Show {
		// handleCLIShow handles the -s/--show option where the configuration is
		// shown, then the program is exited.
		//
		// Exit with success because if the configuration is broken it will be
		// very hard to debug where the problem originates.  This way you can
		// see the configuration and then run the service with the same
		// configuration to see the error.

		fmt.Fprintln(os.Stdout, gs.Explain().String())

		out, err := gs.Marshal()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Fprintln(os.Stdout, "## Final Configuration\n---\n"+string(out))
		}

		os.Exit(0)
	}

	var tmp Config
	err = gs.Unmarshal(goschtalt.Root, &tmp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "There is a critical error in the configuration.")
		fmt.Fprintln(os.Stderr, "Run with -s/--show to see the configuration.")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		// Exit here to prevent a very difficult to debug error from occurring.
		os.Exit(-1)
	}

	return gs, nil
}

// -----------------------------------------------------------------------------
// Keep the default configuration at the bottom of the file so it is easy to
// see what the default configuration is.
// -----------------------------------------------------------------------------

// TODO: update default values to match what's expected of caduceus
var defaultConfig = Config{
	Servers: Servers{
		Health: HealthServer{
			HTTP: arrangehttp.ServerConfig{
				Network: "tcp",
				Address: ":80",
			},
			Path: HealthPath("/"),
		},
		Metrics: MetricsServer{
			HTTP: arrangehttp.ServerConfig{
				Network: "tcp",
				Address: "127.0.0.1:9361",
			},
			Path: MetricsPath("/metrics"),
		},
		Pprof: PprofServer{
			HTTP: arrangehttp.ServerConfig{
				Network: "tcp",
				Address: "127.0.0.1:9999",
			},
			Path: arrangepprof.DefaultPathPrefix,
		},
		Primary: PrimaryServer{
			HTTP: arrangehttp.ServerConfig{
				Network: "tcp",
				Address: ":443",
			},
		},
		Alternate: PrimaryServer{
			HTTP: arrangehttp.ServerConfig{
				Network: "tcp",
				Address: "127.0.0.1:8443",
			},
		},
	},
	Prometheus: touchstone.Config{
		DefaultNamespace: "xmidt",
		DefaultSubsystem: "caduceus",
	},
	Tracing: candlelight.Config{
		ApplicationName: applicationName,
	},
}
