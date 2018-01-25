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

	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/secure"
	"github.com/Comcast/webpa-common/secure/handler"
	"github.com/Comcast/webpa-common/secure/key"
	"github.com/Comcast/webpa-common/server"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/Comcast/webpa-common/webhook/aws"
	"github.com/SermoDigital/jose/jwt"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	applicationName = "caduceus"
	DEFAULT_KEY_ID  = "current"
)

// getValidator returns validator for JWT tokens
func getValidator(v *viper.Viper) (validator secure.Validator, err error) {
	default_validators := make(secure.Validators, 0, 0)
	var jwtVals []JWTValidator

	v.UnmarshalKey("jwtValidators", &jwtVals)

	// make sure there is at least one jwtValidator supplied
	if len(jwtVals) < 1 {
		validator = default_validators
		return
	}

	// if a JWTKeys section was supplied, configure a JWS validator
	// and append it to the chain of validators
	validators := make(secure.Validators, 0, len(jwtVals))

	for _, validatorDescriptor := range jwtVals {
		var keyResolver key.Resolver
		keyResolver, err = validatorDescriptor.Keys.NewResolver()
		if err != nil {
			validator = validators
			return
		}

		validators = append(
			validators,
			secure.JWSValidator{
				DefaultKeyId:  DEFAULT_KEY_ID,
				Resolver:      keyResolver,
				JWTValidators: []*jwt.Validator{validatorDescriptor.Custom.New()},
			},
		)
	}

	// TODO: This should really be part of the unmarshalled validators somehow
	basicAuth := v.GetStringSlice("authHeader")
	for _, authValue := range basicAuth {
		validators = append(
			validators,
			secure.ExactMatchValidator(authValue),
		)
	}

	validator = validators

	return
}

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

	workerPool := WorkerPoolFactory{
		NumWorkers: caduceusConfig.NumWorkerThreads,
		QueueSize:  caduceusConfig.JobQueueSize,
	}.New()

	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		MaxIdleConnsPerHost:   caduceusConfig.SenderNumWorkersPerSender,
		ResponseHeaderTimeout: 10 * time.Second, // TODO Make this configurable
	}

	timeout := time.Duration(caduceusConfig.SenderClientTimeout) * time.Second

	caduceusSenderWrapper, err := SenderWrapperFactory{
		NumWorkersPerSender: caduceusConfig.SenderNumWorkersPerSender,
		QueueSizePerSender:  caduceusConfig.SenderQueueSizePerSender,
		CutOffPeriod:        time.Duration(caduceusConfig.SenderCutOffPeriod) * time.Second,
		Linger:              time.Duration(caduceusConfig.SenderLinger) * time.Second,
		MetricsRegistry:     metricsRegistry,
		Logger:              logger,
		Client:              &http.Client{Transport: tr, Timeout: timeout},
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
		errorRequests:      metricsRegistry.NewCounter(ErrorRequestBodyCounter),
		emptyRequests:      metricsRegistry.NewCounter(EmptyRequestBodyCounter),
		incomingQueueDepth: metricsRegistry.NewGauge(IncomingQueueDepth),
		doJob:              workerPool.Send,
	}

	validator, err := getValidator(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validator error: %v\n", err)
		return 1
	}

	authHandler := handler.AuthorizationHandler{
		HeaderName:          "Authorization",
		ForbiddenStatusCode: 403,
		Validator:           validator,
		Logger:              logger,
	}

	caduceusHandler := alice.New(authHandler.Decorate)

	router := mux.NewRouter()

	router = configServerRouter(router, caduceusHandler, serverWrapper)

	webhookFactory, err := webhook.NewFactory(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating new webhook factory: %s\n", err)
		return 1
	}
	webhookRegistry, webhookHandler := webhookFactory.NewRegistryAndHandler(metricsRegistry)
	webhookFactory.SetExternalUpdate(caduceusSenderWrapper.Update)

	// register webhook end points for api
	router.Handle("/hook", caduceusHandler.ThenFunc(webhookRegistry.UpdateRegistry))
	router.Handle("/hooks", caduceusHandler.ThenFunc(webhookRegistry.GetRegistry))

	selfURL := &url.URL{
		Scheme: "https",
		Host:   v.GetString("fqdn") + v.GetString("primary.address"),
	}

	webhookFactory.Initialize(router, selfURL, webhookHandler, logger, metricsRegistry, nil)

	_, runnable := webPA.Prepare(logger, nil, metricsRegistry, router)

	waitGroup, shutdown, err := concurrent.Execute(runnable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device manager: %s\n", err)
		return 1
	}

	var messageKey = logging.MessageKey()

	debugLog.Log(messageKey, "Calling webhookFactory.PrepareAndStart")
	beginPrepStart := time.Now()
	webhookFactory.PrepareAndStart()
	debugLog.Log(messageKey, "WebhookFactory.PrepareAndStart done.", "elapsedTime", time.Since(beginPrepStart))

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
	s := server.SignalWait(infoLog, signals, os.Interrupt, os.Kill)
	errorLog.Log(logging.MessageKey(), "exiting due to signal", "signal", s)
	close(shutdown)
	waitGroup.Wait()

	// shutdown the sender wrapper gently so that all queued messages get serviced
	caduceusSenderWrapper.Shutdown(true)
	return 0
}

func configServerRouter(router *mux.Router, caduceusHandler alice.Chain, serverWrapper *ServerHandler) *mux.Router {
	var singleContentType = func(r *http.Request, _ *mux.RouteMatch) bool {
		return len(r.Header["Content-Type"]) == 1 //require single specification for Content-Type Header
	}

	router.Handle("/api/v3/notify", caduceusHandler.Then(serverWrapper)).Methods("POST").
		HeadersRegexp("Content-Type", "application/(json|msgpack)").MatcherFunc(singleContentType)

	// Support the old endpoint too.
	router.Handle("/api/v2/notify/{deviceid}/event/{eventtype:.*}", caduceusHandler.Then(serverWrapper)).
		Methods("POST").HeadersRegexp("Content-Type", "application/(json|msgpack)").
		MatcherFunc(singleContentType)

	return router
}

func main() {
	os.Exit(caduceus(os.Args))
}
