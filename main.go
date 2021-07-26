/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

//
import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/webpa-common/concurrent"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/server"
	"github.com/xmidt-org/webpa-common/service/servicecfg"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	applicationName  = "caduceus"
	DEFAULT_KEY_ID   = "current"
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

		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, arguments, f, v, Metrics, ancla.Metrics)
	)

	if parseErr, done := printVersion(f, arguments); done {
		// if we're done, we're exiting no matter what
		if parseErr != nil {
			friendlyError := fmt.Sprintf("failed to parse arguments. detailed error: %s", parseErr)
			logger.Log(
				level.Key(), level.ErrorValue(),
				logging.ErrorKey(), friendlyError)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize Viper environment: %s\n", err)
		return 1
	}

	level.Info(logger).Log("configurationFile", v.ConfigFileUsed())

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
	level.Info(logger).Log(logging.MessageKey(), "tracing status", "enabled", !tracing.IsNoop())

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
		NumWorkersPerSender: caduceusConfig.Sender.NumWorkersPerSender,
		QueueSizePerSender:  caduceusConfig.Sender.QueueSizePerSender,
		CutOffPeriod:        caduceusConfig.Sender.CutOffPeriod,
		Linger:              caduceusConfig.Sender.Linger,
		DeliveryRetries:     caduceusConfig.Sender.DeliveryRetries,
		DeliveryInterval:    caduceusConfig.Sender.DeliveryInterval,
		RetryCodes:          caduceusConfig.Sender.RetryCodes,
		MetricsRegistry:     metricsRegistry,
		Logger:              logger,
		Sender: (&http.Client{
			Transport: tr,
			Timeout:   caduceusConfig.Sender.ClientTimeout,
		}).Do,
	}.New()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize new caduceus sender wrapper: %s\n", err)
		return 1
	}

	serverWrapper := &ServerHandler{
		Logger: logger,
		caduceusHandler: &CaduceusHandler{
			senderWrapper: caduceusSenderWrapper,
			Logger:        logger,
		},
		errorRequests:            metricsRegistry.NewCounter(ErrorRequestBodyCounter),
		emptyRequests:            metricsRegistry.NewCounter(EmptyRequestBodyCounter),
		invalidCount:             metricsRegistry.NewCounter(DropsDueToInvalidPayload),
		incomingQueueDepthMetric: metricsRegistry.NewGauge(IncomingQueueDepth),
		modifiedWRPCount:         metricsRegistry.NewCounter(ModifiedWRPCounter),
		maxOutstanding:           0,
	}

	caduceusConfig.Webhook.Logger = logger
	caduceusConfig.Webhook.MetricsProvider = metricsRegistry
	argusClientTimeout, err := newArgusClientTimeout(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse argus client timeout config values: %v \n", err)
		return 1
	}

	caduceusConfig.Webhook.Argus.HTTPClient = newHTTPClient(argusClientTimeout, tracing)
	svc, stopWatches, err := ancla.Initialize(caduceusConfig.Webhook, getLogger, logging.WithLogger, caduceusSenderWrapper)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Webhook service initialization error: %v\n", err)
		return 1
	}
	level.Info(logger).Log(logging.MessageKey(), "Webhook service enabled")

	rootRouter := mux.NewRouter()
	otelMuxOptions := []otelmux.Option{
		otelmux.WithPropagators(tracing.Propagator()),
		otelmux.WithTracerProvider(tracing.TracerProvider()),
	}
	rootRouter.Use(otelmux.Middleware("primary", otelMuxOptions...), candlelight.EchoFirstTraceNodeInfo(tracing.Propagator()))

	primaryHandler, err := NewPrimaryHandler(logger, v, serverWrapper, svc, metricsRegistry, rootRouter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validator error: %v\n", err)
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
		level.Info(logger).Log(logging.MessageKey(), "no service discovery configured")
	} else {
		e, err := servicecfg.NewEnvironment(logger, v.Sub("service"))
		if err != nil {
			level.Error(logger).Log(logging.MessageKey(), "Unable to initialize service discovery environment", logging.ErrorKey(), err)
			return 4
		}

		defer e.Close()
		level.Info(logger).Log("configurationFile", v.ConfigFileUsed())
		e.Register()
	}

	level.Info(logger).Log(logging.MessageKey(), "Caduceus is up and running!", "elapsedTime", time.Since(beginCaduceus))

	signals := make(chan os.Signal, 10)
	signal.Notify(signals, os.Kill, os.Interrupt)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			level.Error(logger).Log(logging.MessageKey(), "exiting due to signal", "signal", s)
			exit = true
		case <-done:
			level.Error(logger).Log(logging.MessageKey(), "one or more servers exited")
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
