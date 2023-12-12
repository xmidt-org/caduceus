// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/alecthomas/kong"
	"github.com/goschtalt/goschtalt"

	_ "github.com/goschtalt/goschtalt/pkg/typical"
	_ "github.com/goschtalt/yaml-decoder"
	_ "github.com/goschtalt/yaml-encoder"
	"github.com/xmidt-org/arrange/arrangehttp"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"
	"github.com/xmidt-org/touchstone/touchhttp"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

const (
	applicationName  = "caduceus"
	defaultKeyID     = "current"
	tracingConfigKey = "tracing"
)

var (
	commit  = "undefined"
	version = "undefined"
	date    = "undefined"
	builtBy = "undefined"
)

type CLI struct {
	Dev   bool     `optional:"" short:"d" help:"Run in development mode."`
	Show  bool     `optional:"" short:"s" help:"Show the configuration and exit."`
	Graph string   `optional:"" short:"g" help:"Output the dependency graph to the specified file."`
	Files []string `optional:"" short:"f" help:"Specific configuration files or directories."`
}

// Provides a named type so it's a bit easier to flow through & use in fx.
type cliArgs []string

func caduceus(arguments []string, run bool) error {
	var (
		gscfg *goschtalt.Config

		// Capture the dependency tree in case we need to debug something.
		g fx.DotGraph

		// Capture the command line arguments.
		cli *CLI
	)
	app := fx.New(
		fx.Supply(cliArgs(arguments)),
		fx.Populate(&g),
		fx.Populate(&gscfg),
		fx.Populate(&cli),

		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log}
		}),

		fx.Provide(
			provideCLI,
			provideLogger,
			provideConfig,
			goschtalt.UnmarshalFunc[sallust.Config]("logging"),
			goschtalt.UnmarshalFunc[candlelight.Config]("tracing"),
			goschtalt.UnmarshalFunc[touchstone.Config]("prometheus"),
			goschtalt.UnmarshalFunc[SenderConfig]("sender"),
			goschtalt.UnmarshalFunc[Service]("service"),
			goschtalt.UnmarshalFunc[[]string]("authHeader"),
			goschtalt.UnmarshalFunc[bool]("previousVersionSupport"),
			goschtalt.UnmarshalFunc[HealthPath]("servers.health.path"),
			goschtalt.UnmarshalFunc[MetricsPath]("servers.metrics.path"),
			goschtalt.UnmarshalFunc[PprofPathPrefix]("servers.pprof.path"),
			fx.Annotated{
				Name:   "server",
				Target: goschtalt.UnmarshalFunc[string]("server"),
			},
			fx.Annotated{
				Name:   "fqdn",
				Target: goschtalt.UnmarshalFunc[string]("fqdn"),
			},
			fx.Annotated{
				Name:   "build",
				Target: goschtalt.UnmarshalFunc[string]("build"),
			},
			fx.Annotated{
				Name:   "flavor",
				Target: goschtalt.UnmarshalFunc[string]("flavor"),
			},
			fx.Annotated{
				Name:   "region",
				Target: goschtalt.UnmarshalFunc[string]("region"),
			},
			fx.Annotated{
				Name:   "servers.health.config",
				Target: goschtalt.UnmarshalFunc[arrangehttp.ServerConfig]("servers.health.http"),
			},
			fx.Annotated{
				Name:   "servers.metrics.config",
				Target: goschtalt.UnmarshalFunc[arrangehttp.ServerConfig]("servers.metrics.http"),
			},
			fx.Annotated{
				Name:   "servers.pprof.config",
				Target: goschtalt.UnmarshalFunc[arrangehttp.ServerConfig]("servers.pprof.http"),
			},
			fx.Annotated{
				Name:   "servers.primary.config",
				Target: goschtalt.UnmarshalFunc[arrangehttp.ServerConfig]("servers.primary.http"),
			},
			fx.Annotated{
				Name:   "servers.alternate.config",
				Target: goschtalt.UnmarshalFunc[arrangehttp.ServerConfig]("servers.alternate.http"),
			},
			candlelight.New,
		),

		providePprofEndpoint(),
		provideMetricEndpoint(),
		provideHealthCheck(),

		arrangehttp.ProvideServer("servers.health"),
		arrangehttp.ProvideServer("servers.metrics"),
		arrangehttp.ProvideServer("servers.pprof"),
		// arrangehttp.ProvideServer("servers.primary"),
		// arrangehttp.ProvideServer("servers.alternate"),

		touchstone.Provide(),
		touchhttp.Provide(),
		ProvideMetrics(),
		// ancla.ProvideMetrics(), //TODO: need to add back in once we fix the ancla/argus dependency issue

	)

	if cli != nil && cli.Graph != "" {
		_ = os.WriteFile(cli.Graph, []byte(g), 0644)
	}

	if cli != nil && cli.Dev {
		defer func() {
			if gscfg != nil {
				fmt.Fprintln(os.Stderr, gscfg.Explain().String())
			}
		}()
	}

	if err := app.Err(); err != nil {
		return err
	}

	if run {
		app.Run()
	}

	return nil
}

func provideCLI(args cliArgs) (*CLI, error) {
	return provideCLIWithOpts(args, false)
}

func provideCLIWithOpts(args cliArgs, testOpts bool) (*CLI, error) {
	var cli CLI

	// Create a no-op option to satisfy the kong.New() call.
	var opt kong.Option = kong.OptionFunc(
		func(*kong.Kong) error {
			return nil
		},
	)

	if testOpts {
		opt = kong.Writers(nil, nil)
	}

	parser, err := kong.New(&cli,
		kong.Name(applicationName),
		kong.Description("The cpe agent for Xmidt service.\n"+
			fmt.Sprintf("\tVersion:  %s\n", version)+
			fmt.Sprintf("\tDate:     %s\n", date)+
			fmt.Sprintf("\tCommit:   %s\n", commit)+
			fmt.Sprintf("\tBuilt By: %s\n", builtBy),
		),
		kong.UsageOnError(),
		opt,
	)
	if err != nil {
		return nil, err
	}

	if testOpts {
		parser.Exit = func(_ int) { panic("exit") }
	}

	_, err = parser.Parse(args)
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	return &cli, nil
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("stacktrace from panic: \n" + string(debug.Stack()))
		}
	}()

	err := caduceus(os.Args[1:], true)

	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, err)
	os.Exit(-1)
}
