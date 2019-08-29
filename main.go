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
	"crypto/tls"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/xmidt-org/webpa-common/service/servicecfg"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/webpa-common/concurrent"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/server"
	"github.com/xmidt-org/webpa-common/webhook"
	"github.com/xmidt-org/webpa-common/webhook/aws"
)

const (
	applicationName    = "caduceus"
	DEFAULT_KEY_ID     = "current"
	applicationVersion = "0.2.0"
)

// caduceus is the driver function for Caduceus.  It performs everything main() would do,
// except for obtaining the command-line arguments (which are passed to it).

func caduceus(arguments []string) int {
	beginCaduceus := time.Now()

	var (
		f = pflag.NewFlagSet(applicationName, pflag.ContinueOnError)
		v = viper.New()

		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, arguments, f, v, Metrics, webhook.Metrics, aws.Metrics)
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize Viper environment: %s\n", err)
		return 1
	}

	var (
		infoLog  = logging.Info(logger)
		errorLog = logging.Error(logger)
		debugLog = logging.Debug(logger)
	)

	infoLog.Log("configurationFile", v.ConfigFileUsed())

	caduceusConfig := new(CaduceusConfig)
	err = v.Unmarshal(caduceusConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to unmarshal configuration data into struct: %s\n", err)
		return 1
	}

	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
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
		maxOutstanding:           0,
	}

	webhookFactory, err := webhook.NewFactory(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating new webhook factory: %s\n", err)
		return 1
	}
	webhookRegistry, webhookHandler := webhookFactory.NewRegistryAndHandler(metricsRegistry)
	webhookFactory.SetExternalUpdate(caduceusSenderWrapper.Update)

	primaryHandler, err := NewPrimaryHandler(logger, v, serverWrapper, &webhookRegistry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validator error: %v\n", err)
		return 1
	}

	scheme := v.GetString("scheme")
	if len(scheme) < 1 {
		scheme = "https"
	}

	selfURL := &url.URL{
		Scheme: scheme,
		Host:   v.GetString("fqdn") + v.GetString("primary.address"),
	}

	webhookFactory.Initialize(primaryHandler, selfURL, v.GetString("soa.provider"), webhookHandler, logger, metricsRegistry, nil)

	_, runnable, done := webPA.Prepare(logger, nil, metricsRegistry, primaryHandler)

	waitGroup, shutdown, err := concurrent.Execute(runnable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device manager: %s\n", err)
		return 1
	}

	var messageKey = logging.MessageKey()

	if webhookFactory != nil {
		// wait for DNS to propagate before subscribing to SNS
		if err = webhookFactory.DnsReady(); err == nil {
			debugLog.Log(messageKey, "Calling webhookFactory.PrepareAndStart. Server is ready to take on subscription confirmations")
			webhookFactory.PrepareAndStart()
		} else {
			errorLog.Log(messageKey, "Server was not ready within a time constraint. SNS confirmation could not happen",
				logging.ErrorKey(), err)
		}
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

	// Attempt to obtain the current listener list from current system without having to wait for listener reregistration.
	debugLog.Log(messageKey, "Attempting to obtain current listener list from source", "source",
		v.GetString("start.apiPath"))
	beginObtainList := time.Now()
	startChan := make(chan webhook.Result, 1)
	webhookFactory.Start.GetCurrentSystemsHooks(startChan)
	var webhookStartResults webhook.Result = <-startChan
	if webhookStartResults.Error != nil {
		errorLog.Log(logging.ErrorKey(), webhookStartResults.Error)
	} else {
		// todo: add message
		webhookFactory.SetList(webhook.NewList(webhookStartResults.Hooks))
		caduceusSenderWrapper.Update(webhookStartResults.Hooks)
	}

	debugLog.Log(messageKey, "Current listener retrieval.", "elapsedTime", time.Since(beginObtainList))
	infoLog.Log(messageKey, "Caduceus is up and running!", "elapsedTime", time.Since(beginCaduceus))

	signals := make(chan os.Signal, 10)
	signal.Notify(signals)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			if s != os.Kill && s != os.Interrupt {
				logger.Log(level.Key(), level.InfoValue(), logging.MessageKey(), "ignoring signal", "signal", s)
			} else {
				logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "exiting due to signal", "signal", s)
				exit = true
			}
		case <-done:
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "one or more servers exited")
			exit = true
		}
	}

	close(shutdown)
	waitGroup.Wait()

	// shutdown the sender wrapper gently so that all queued messages get serviced
	caduceusSenderWrapper.Shutdown(true)
	return 0
}

func main() {
	os.Exit(caduceus(os.Args))
}
