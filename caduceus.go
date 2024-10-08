// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package caduceus

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/alecthomas/kong"
	"github.com/goschtalt/goschtalt"

	_ "github.com/goschtalt/goschtalt/pkg/typical"
	_ "github.com/goschtalt/yaml-decoder"
	_ "github.com/goschtalt/yaml-encoder"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/ancla/anclafx"
	"github.com/xmidt-org/arrange/arrangehttp"
	"github.com/xmidt-org/caduceus/internal/client"
	"github.com/xmidt-org/caduceus/internal/handler"
	"github.com/xmidt-org/caduceus/internal/metrics"
	"github.com/xmidt-org/caduceus/internal/sink"
	"github.com/xmidt-org/clortho/clorthofx"

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

type LifeCycleIn struct {
	fx.In
	Logger     *zap.Logger
	LC         fx.Lifecycle
	Shutdowner fx.Shutdowner
}
type CLI struct {
	Dev   bool     `optional:"" short:"d" help:"Run in development mode."`
	Show  bool     `optional:"" short:"s" help:"Show the configuration and exit."`
	Graph string   `optional:"" short:"g" help:"Output the dependency graph to the specified file."`
	Files []string `optional:"" short:"f" help:"Specific configuration files or directories."`
}

// Provides a named type so it's a bit easier to flow through & use in fx.
type cliArgs []string

func Caduceus(arguments []string, run bool) error {
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
			goschtalt.UnmarshalFunc[handler.CapabilityConfig]("capabilityCheck"),
			goschtalt.UnmarshalFunc[sink.Config]("sender"),
			goschtalt.UnmarshalFunc[Service]("service"),
			goschtalt.UnmarshalFunc[client.HttpClientTimeout]("argusClientTimeout"),
			goschtalt.UnmarshalFunc[[]string]("authHeader"),
			goschtalt.UnmarshalFunc[bool]("previousVersionSupport"),
			goschtalt.UnmarshalFunc[HealthPath]("servers.health.path"),
			goschtalt.UnmarshalFunc[MetricsPath]("servers.metrics.path"),
			goschtalt.UnmarshalFunc[PprofPathPrefix]("servers.pprof.path"),
			goschtalt.UnmarshalFunc[ancla.Config]("webhook"),
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

		anclafx.Provide(),
		providePprofEndpoint(),
		provideMetricEndpoint(),
		provideHealthCheck(),
		provideCoreEndpoints(),

		arrangehttp.ProvideServer("servers.health"),
		arrangehttp.ProvideServer("servers.metrics"),
		arrangehttp.ProvideServer("servers.pprof"),
		arrangehttp.ProvideServer("servers.primary"),
		arrangehttp.ProvideServer("servers.alternate"),

		handler.Provide(),
		sink.Provide(),
		touchstone.Provide(),
		touchhttp.Provide(),
		metrics.Provide(),
		client.Provide(),
		clorthofx.Provide(),

		fx.Invoke(
			lifeCycle,
		),
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
		kong.Description("Xmidt Caudceus service.\n"),
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

func onStart(logger *zap.Logger) func(context.Context) error {
	logger = logger.Named("on_start")

	return func(ctx context.Context) error {
		defer func() {
			if r := recover(); nil != r {
				logger.Error("stacktrace from panic", zap.String("stacktrace", string(debug.Stack())), zap.Any("panic", r))
			}
		}()

		return nil
	}
}

func onStop(shutdowner fx.Shutdowner, logger *zap.Logger) func(context.Context) error {
	logger = logger.Named("on_stop")

	return func(_ context.Context) error {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("stacktrace from panic", zap.String("stacktrace", string(debug.Stack())), zap.Any("panic", r))
			}

			if err := shutdowner.Shutdown(); err != nil {
				logger.Error("encountered error trying to shutdown app: ", zap.Error(err))
			}
		}()

		return nil
	}
}

func lifeCycle(in LifeCycleIn) {
	logger := in.Logger.Named("fx_lifecycle")
	in.LC.Append(
		fx.Hook{
			OnStart: onStart(logger),
			OnStop:  onStop(in.Shutdowner, logger),
		},
	)
}
