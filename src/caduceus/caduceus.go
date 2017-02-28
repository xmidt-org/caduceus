// TODO: figure out how to use the config file in this situation and adjust this file and caduceus_type.go accordingly

package main

import (
	"fmt"
	"github.com/Comcast/webpa-common/concurrent"
	// "github.com/Comcast/webpa-common/secure"
	"github.com/Comcast/webpa-common/server"
	// "github.com/justinas/alice"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"
	"os/signal"
)

const (
	applicationName = "caduceus"
)

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

	logger.Info("Caduceus is up and running!")
	logger.Info("Finished reading config file and generating logger!")

	serverWrapper := &ServerHandler{
		logger: logger,
		workerPool: WorkerPoolFactory{
			NumWorkers: 100,
			QueueSize:  10,
			// NumWorkers: inConfig.NumWorkerThreads,
			// QueueSize:  inConfig.JobQueueSize,
		}.New(),
	}

	// validator := secure.Validators{
	// 	secure.ExactMatchValidator(inConfig.AuthHeader),
	// }

	// authHandler := handler.AuthorizationHandler{
	// 	HeaderName:          "Authorization",
	// 	ForbiddenStatusCode: 403,
	// 	Validator:           validator,
	// 	Logger:              inLogger,
	// }

	// caduceusHandler := alice.New(authHandler.Decorate)

	_, runnable := webPA.Prepare(logger, serverWrapper)
	waitGroup, shutdown, err := concurrent.Execute(runnable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device manager: %s\n", err)
		return 1
	}

	var (
		signals = make(chan os.Signal, 1)
	)

	signal.Notify(signals)
	<-signals
	close(shutdown)
	waitGroup.Wait()

	return 0
}

func main() {
	os.Exit(caduceus(os.Args))
}
