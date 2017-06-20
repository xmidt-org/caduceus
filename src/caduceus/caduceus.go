package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/secure"
	"github.com/Comcast/webpa-common/secure/handler"
	"github.com/Comcast/webpa-common/secure/key"
	"github.com/Comcast/webpa-common/server"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/SermoDigital/jose/jwt"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"
)

const (
	applicationName = "caduceus"
	DEFAULT_KEY_ID = "current"
)


// getValidator returns validator for JWT tokens
func getValidator(v *viper.Viper) (validator secure.Validator, err error) {
	no_validators := make(secure.Validators, 0, 0)
	var jwtVals []JWTValidator
	var ok bool
	
	// obtain valid jwtValidator from configuration
	if jwtVals, ok = v.Get("jwtValidators").([]JWTValidator); !ok {
		validator = no_validators
		err = errors.New("Unable to cast \"jwtValidators\" value as type []JWTValidator")
		return
	}

	// make sure there is at least one jwtValidator supplied
	if len(jwtVals) < 1 {
		validator = no_validators
		err = errors.New("Supplied jwtValidator list size is less than 1")
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

	validator = validators
	
	return
}


// caduceus is the driver function for Caduceus.  It performs everything main() would do,
// except for obtaining the command-line arguments (which are passed to it).
func caduceus(arguments []string) int {
	var (
		f = pflag.NewFlagSet(applicationName, pflag.ContinueOnError)
		v = viper.New()

		logger, webPA, err = server.Initialize(applicationName, arguments, f, v)
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize Viper environment: %s\n", err)
		return 1
	}

	logger.Info("Using configuration file: %s", v.ConfigFileUsed())

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

	caduceusProfilerFactory := ServerProfilerFactory{
		Frequency: caduceusConfig.ProfilerFrequency,
		Duration:  caduceusConfig.ProfilerDuration,
		QueueSize: caduceusConfig.ProfilerQueueSize,
	}
	
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	timeout := time.Duration(caduceusConfig.SenderClientTimeout) * time.Second

	// declare a new sender wrapper and pass it a profiler factory so that it can create
	// unique profilers on a per outboundSender basis
	// TODO: need to add `Update` method into the code
	caduceusSenderWrapper, err := SenderWrapperFactory{
		NumWorkersPerSender: caduceusConfig.SenderNumWorkersPerSender,
		QueueSizePerSender:  caduceusConfig.SenderQueueSizePerSender,
		CutOffPeriod:        time.Duration(caduceusConfig.SenderCutOffPeriod) * time.Second,
		Linger:              time.Duration(caduceusConfig.SenderLinger) * time.Second,
		ProfilerFactory:     caduceusProfilerFactory,
		Logger:              logger,
		Client:              &http.Client{Transport: tr, Timeout: timeout},
	}.New()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize new caduceus sender wrapper: %s\n", err)
		return 1
	}

	// here we create a profiler specifically for our main server handler
	caduceusHandlerProfiler, err := caduceusProfilerFactory.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to profiler for main caduceus handler: %s\n", err)
		return 1
	}

	serverWrapper := &ServerHandler{
		Logger: logger,
		caduceusHandler: &CaduceusHandler{
			handlerProfiler: caduceusHandlerProfiler,
			senderWrapper:   caduceusSenderWrapper,
			Logger:          logger,
		},
		doJob: workerPool.Send,
	}

	profileWrapper := &ProfileHandler{
		profilerData: caduceusHandlerProfiler,
		Logger:       logger,
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

	mux := mux.NewRouter()
	mux.Handle("/api/v1/run", caduceusHandler.Then(serverWrapper))
	mux.Handle("/api/v1/profile", caduceusHandler.Then(profileWrapper))



	webhookFactory, err := webhook.NewFactory(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating new webhook factory: %s\n", err)
		return 1
	}
	
	_, webhookHandler := webhookFactory.NewListAndHandler()
	webhookRegistry := webhook.NewRegistry(nil, webhookFactory.PublishMessage)
	webhookFactory.SetList( webhookRegistry )

	// register webhook end points for api
	mux.Handle("/hook", caduceusHandler.ThenFunc(webhookRegistry.UpdateRegistry))
	mux.Handle("/hooks", caduceusHandler.ThenFunc(webhookRegistry.GetRegistry))
	
	selfURL := &url.URL{
		Scheme:   "https",
		Host:     v.GetString("fqdn") + v.GetString("primary.address"),
	}
	
	webhookFactory.Initialize(mux, selfURL, webhookHandler, logger)
	webhookFactory.PrepareAndStart()

	caduceusHealth := &CaduceusHealth{}
	var runnable concurrent.Runnable

	caduceusHealth.Monitor, runnable = webPA.Prepare(logger, mux)
	serverWrapper.caduceusHealth = caduceusHealth

	waitGroup, shutdown, err := concurrent.Execute(runnable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device manager: %s\n", err)
		return 1
	}

	logger.Info("Caduceus is up and running!")

	// Attempt to obtain the current listener list from current system without having to wait for listener reregistration.
	startChan := make(chan webhook.Result, 1)
	webhookFactory.Start.GetCurrentSystemsHooks(startChan)
	var webhookStartResults webhook.Result = <- startChan
	if webhookStartResults.Error != nil {
		logger.Error(webhookStartResults.Error)
	} else {
		webhookRegistry.Update(webhookStartResults.Hooks)
	}

	var (
		signals = make(chan os.Signal, 1)
	)

	signal.Notify(signals)
	<-signals
	close(shutdown)
	waitGroup.Wait()

	// shutdown the sender wrapper gently so that all queued messages get serviced
	caduceusSenderWrapper.Shutdown(true)

	return 0
}

func main() {
	os.Exit(caduceus(os.Args))
}
