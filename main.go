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

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/argus/chrysom"
	"github.com/xmidt-org/argus/model"
	"github.com/xmidt-org/webpa-common/webhook"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/xmidt-org/webpa-common/service/servicecfg"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/webpa-common/concurrent"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/server"
	"github.com/xmidt-org/webpa-common/webhook/aws"
)

const (
	applicationName = "caduceus"
	DEFAULT_KEY_ID  = "current"
)

var (
	GitCommit = "undefined"
	Version   = "undefined"
	BuildTime = "undefined"
)

// caduceus is the driver function for Caduceus.  It performs everything main() would do,
// except for obtaining the command-line arguments (which are passed to it).

func caduceus(arguments []string) int {
	beginCaduceus := time.Now()

	var (
		f = pflag.NewFlagSet(applicationName, pflag.ContinueOnError)
		v = viper.New()

		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, arguments, f, v, Metrics, aws.Metrics, chrysom.Metrics)
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

	log.WithPrefix(logger, level.Key(), level.InfoValue()).Log("configurationFile", v.ConfigFileUsed())

	caduceusConfig := new(CaduceusConfig)
	err = v.Unmarshal(caduceusConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to unmarshal configuration data into struct: %s\n", err)
		return 1
	}

	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: caduceusConfig.Sender.DisableClientHostnameValidation},
		MaxIdleConnsPerHost:   caduceusConfig.Sender.NumWorkersPerSender,
		ResponseHeaderTimeout: caduceusConfig.Sender.ResponseHeaderTimeout,
		IdleConnTimeout:       caduceusConfig.Sender.IdleConnTimeout,
	}

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
	measures := NewMeasures(metricsRegistry)

	caduceusConfig.WebhookStore.Logger = logger
	caduceusConfig.WebhookStore.HTTPClient = http.DefaultClient
	var webhookListener chrysom.ListenerFunc = func(items []model.Item) {
		hooks := []webhook.W{}
		for _, item := range items {
			hook, err := convertItemToWebhook(item)
			if err != nil {
				log.WithPrefix(logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Webhook", "item", item)
				continue
			}
			hooks = append(hooks, hook)
		}
		caduceusSenderWrapper.Update(hooks)
		// Updated webhook list size metric for legacy metrics
		measures.WebhookListSize.Set(float64(len(items)))
	}
	caduceusConfig.WebhookStore.Listener = webhookListener
	caduceusConfig.WebhookStore.MetricsProvider = metricsRegistry

	webhookRegistry, err := NewRegistry(caduceusConfig.WebhookStore)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Validator error: %v\n", err)
		return 1
	}

	primaryHandler, err := NewPrimaryHandler(logger, v, serverWrapper, webhookRegistry)
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
	if false == v.IsSet("service") {
		logger.Log(level.Key(), level.InfoValue(), logging.MessageKey(), "no service discovery configured")
	} else {
		e, err := servicecfg.NewEnvironment(logger, v.Sub("service"))
		if err != nil {
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to initialize service discovery environment", logging.ErrorKey(), err)
			return 4
		}

		defer e.Close()
		logger.Log(level.Key(), level.InfoValue(), "configurationFile", v.ConfigFileUsed())
		e.Register()
	}

	log.WithPrefix(logger, level.Key(), level.InfoValue()).Log(logging.MessageKey(), "Caduceus is up and running!", "elapsedTime", time.Since(beginCaduceus))

	signals := make(chan os.Signal, 10)
	signal.Notify(signals, os.Kill, os.Interrupt)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "exiting due to signal", "signal", s)
			exit = true
		case <-done:
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "one or more servers exited")
			exit = true
		}
	}

	close(shutdown)
	waitGroup.Wait()

	// shutdown the sender wrapper gently so that all queued messages get serviced
	caduceusSenderWrapper.Shutdown(true)
	webhookRegistry.Stop(context.Background())
	return 0
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
