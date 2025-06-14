// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

//
import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/bascule/basculehelper"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/httpaux/recovery"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"

	"github.com/xmidt-org/webpa-common/v2/adapter"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/concurrent"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/server"
	"github.com/xmidt-org/webpa-common/v2/service/servicecfg"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	applicationName  = "caduceus"
	defaultKeyID     = "current"
	tracingConfigKey = "tracing"
)

var (
	GitCommit = "undefined"
	Version   = "undefined"
	BuildTime = "undefined"
)

// httpClientTimeout contains timeouts for an HTTP client and its requests.
type httpClientTimeout struct {
	// ClientTimeout is HTTP Client Timeout.
	ClientTimeout time.Duration

	// NetDialerTimeout is the net dialer timeout
	NetDialerTimeout time.Duration
}

// caduceus is the driver function for Caduceus.  It performs everything main() would do,
// except for obtaining the command-line arguments (which are passed to it).
func caduceus(arguments []string) int {
	beginCaduceus := time.Now()

	var (
		f = pflag.NewFlagSet(applicationName, pflag.ContinueOnError)
		v = viper.New()

		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, arguments, f, v, basculehelper.AuthCapabilitiesMetrics, basculehelper.AuthValidationMetrics)
	)

	promReg, ok := metricsRegistry.(prometheus.Registerer)
	if !ok {
		fmt.Fprintf(os.Stderr, "failed to get prometheus registerer")
		return 1
	}

	var tsConfig touchstone.Config
	// Get touchstone & zap configurations
	v.UnmarshalKey("touchstone", &tsConfig)
	tf := touchstone.NewFactory(tsConfig, logger, promReg)
	serverHandlerMetrics, senderWrapperMetrics, outboundSenderMetrics, anclaListnerMetric, err := Metrics(tf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed metrics setup: %s\n", err)
		return 1
	}

	if parseErr, done := printVersion(f, arguments); done {
		// if we're done, we're exiting no matter what
		if parseErr != nil {
			friendlyError := fmt.Sprintf("failed to parse arguments. detailed error: %s", parseErr)
			logger.Error(friendlyError)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize Viper environment: %s\n", err)
		return 1
	}

	logger.Info("viper environment successfully initialized", zap.Any("configurationFile", v.ConfigFileUsed()))

	caduceusConfig := new(CaduceusConfig)
	err = v.Unmarshal(caduceusConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to unmarshal configuration data into struct: %s\n", err)
		return 1
	}

	tracing, err := loadTracing(v, applicationName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to build tracing component: %v \n", err)
		return 1
	}
	logger.Info("tracing status", zap.Bool("enabled", !tracing.IsNoop()))

	var tr http.RoundTripper = &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: caduceusConfig.Sender.DisableClientHostnameValidation},
		MaxIdleConnsPerHost:   caduceusConfig.Sender.NumWorkersPerSender,
		ResponseHeaderTimeout: caduceusConfig.Sender.ResponseHeaderTimeout,
		IdleConnTimeout:       caduceusConfig.Sender.IdleConnTimeout,
	}

	tr = otelhttp.NewTransport(tr,
		otelhttp.WithPropagators(tracing.Propagator()),
		otelhttp.WithTracerProvider(tracing.TracerProvider()),
	)

	caduceusSenderWrapper, err := SenderWrapperFactory{
		NumWorkersPerSender:   caduceusConfig.Sender.NumWorkersPerSender,
		QueueSizePerSender:    caduceusConfig.Sender.QueueSizePerSender,
		CutOffPeriod:          caduceusConfig.Sender.CutOffPeriod,
		Linger:                caduceusConfig.Sender.Linger,
		DeliveryRetries:       caduceusConfig.Sender.DeliveryRetries,
		DeliveryInterval:      caduceusConfig.Sender.DeliveryInterval,
		Metrics:               senderWrapperMetrics,
		outboundSenderMetrics: outboundSenderMetrics,
		Logger:                logger,
		Sender: doerFunc((&http.Client{
			Transport: tr,
			Timeout:   caduceusConfig.Sender.ClientTimeout,
		}).Do),
		CustomPIDs:        caduceusConfig.Sender.CustomPIDs,
		DisablePartnerIDs: caduceusConfig.Sender.DisablePartnerIDs,
	}.New()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize new caduceus sender wrapper: %s\n", err)
		return 1
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create %v metric: %v \n", IncomingQueueLatencyHistogram, err)
		return 1
	}

	serverWrapper := &ServerHandler{
		Logger: logger,
		caduceusHandler: &CaduceusHandler{
			senderWrapper: caduceusSenderWrapper,
			Logger:        logger,
		},
		metrics:        serverHandlerMetrics,
		maxOutstanding: 0,
		now:            time.Now,
	}

	caduceusConfig.Webhook.Logger = logger
	caduceusConfig.Listener.Measures = anclaListnerMetric
	argusClientTimeout, err := newArgusClientTimeout(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse argus client timeout config values: %v \n", err)
		return 1
	}

	caduceusConfig.Webhook.BasicClientConfig.HTTPClient = newHTTPClient(argusClientTimeout, tracing)
	svc, err := ancla.NewService(caduceusConfig.Webhook, getLogger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Webhook service initialization error: %v\n", err)
		return 1
	}
	stopWatches, err := svc.StartListener(caduceusConfig.Listener, setLoggerInContext(), caduceusSenderWrapper)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Webhook service start listener error: %v\n", err)
		return 1
	}

	logger.Info("Webhook service enabled")
	rootRouter := mux.NewRouter()
	rootRouter.Use(
		recovery.Middleware(
			recovery.WithStatusCode(555), // a wacky status code that will show up in metrics
			// TODO: should probably customize things a bit
		),
	)

	otelMuxOptions := []otelmux.Option{
		otelmux.WithPropagators(tracing.Propagator()),
		otelmux.WithTracerProvider(tracing.TracerProvider()),
	}
	rootRouter.Use(otelmux.Middleware("primary", otelMuxOptions...), candlelight.EchoFirstTraceNodeInfo(tracing, false))

	primaryHandler, err := NewPrimaryHandler(logger, v, metricsRegistry, tf, serverWrapper, svc, rootRouter, v.GetBool("previousVersionSupport"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Handler creation error: %v\n", err)
		return 1
	}

	_, runnable, done := webPA.Prepare(logger, nil, metricsRegistry, primaryHandler)

	waitGroup, shutdown, err := concurrent.Execute(runnable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device manager: %s\n", err)
		return 1
	}

	//
	// Now, initialize the service discovery infrastructure
	//
	if !v.IsSet("service") {
		logger.Info("no service discovery configured")
	} else {
		var log = &adapter.Logger{
			Logger: logger,
		}
		e, err := servicecfg.NewEnvironment(log, v.Sub("service"))
		if err != nil {
			logger.Error("Unable to initialize service discovery environment", zap.Error(err))
			return 4
		}

		defer e.Close()
		logger.Info("service discovery environment successfully initialized", zap.Any("configurationFile", v.ConfigFileUsed()))
		e.Register()
	}

	logger.Info("Caduceus is up and running!", zap.Any("elapsedTime", time.Since(beginCaduceus)))

	signals := make(chan os.Signal, 10)
	signal.Notify(signals, syscall.SIGTERM, os.Interrupt)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			logger.Error("exiting due to signal", zap.Any("signal", s))
			exit = true
		case <-done:
			logger.Error("one or more servers exited")
			exit = true
		}
	}

	close(shutdown)
	waitGroup.Wait()

	// shutdown the sender wrapper gently so that all queued messages get serviced
	caduceusSenderWrapper.Shutdown(true)
	stopWatches()
	return 0
}

func setLoggerInContext() func(context.Context, *zap.Logger) context.Context {
	return func(parent context.Context, logger *zap.Logger) context.Context {
		return sallust.With(parent, logger)
	}
}

func loadTracing(v *viper.Viper, appName string) (candlelight.Tracing, error) {
	var traceConfig candlelight.Config
	err := v.UnmarshalKey(tracingConfigKey, &traceConfig)
	if err != nil {
		return candlelight.Tracing{}, err
	}
	traceConfig.ApplicationName = appName
	tracing, err := candlelight.New(traceConfig)
	return tracing, err
}

func newArgusClientTimeout(v *viper.Viper) (httpClientTimeout, error) {
	var timeouts httpClientTimeout
	err := v.UnmarshalKey("argusClientTimeout", &timeouts)
	if err != nil {
		return httpClientTimeout{}, err
	}
	if timeouts.ClientTimeout == 0 {
		timeouts.ClientTimeout = time.Second * 50
	}
	if timeouts.NetDialerTimeout == 0 {
		timeouts.NetDialerTimeout = time.Second * 5
	}
	return timeouts, nil

}

func newHTTPClient(timeouts httpClientTimeout, tracing candlelight.Tracing) *http.Client {
	var transport http.RoundTripper = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: timeouts.NetDialerTimeout,
		}).Dial,
	}

	transport = otelhttp.NewTransport(transport,
		otelhttp.WithPropagators(tracing.Propagator()),
		otelhttp.WithTracerProvider(tracing.TracerProvider()),
	)

	return &http.Client{
		Timeout:   timeouts.ClientTimeout,
		Transport: transport,
	}
}

func printVersion(f *pflag.FlagSet, arguments []string) (error, bool) {
	printVer := f.BoolP("version", "v", false, "displays the version number")
	if err := f.Parse(arguments); err != nil {
		return err, true
	}

	if *printVer {
		printVersionInfo(os.Stdout)
		return nil, true
	}
	return nil, false
}

func printVersionInfo(writer io.Writer) {
	fmt.Fprintf(writer, "%s:\n", applicationName)
	fmt.Fprintf(writer, "  version: \t%s\n", Version)
	fmt.Fprintf(writer, "  go version: \t%s\n", runtime.Version())
	fmt.Fprintf(writer, "  built time: \t%s\n", BuildTime)
	fmt.Fprintf(writer, "  git commit: \t%s\n", GitCommit)
	fmt.Fprintf(writer, "  os/arch: \t%s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func main() {
	os.Exit(caduceus(os.Args))
}
