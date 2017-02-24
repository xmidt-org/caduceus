package main

import (
	"flag"
	"fmt"
	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/handler"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/secure"
	"github.com/Comcast/webpa-common/server"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"os"
	"os/signal"
)

const (
	DEFAULT_SERVER_CONFIG = "caduceus.cfg.json"
)

// LoadConfiguration goes through the config file provided and then loads it into a struct for later use
func LoadConfiguration(serverConfigFilename string) (configuration *Configuration, logger logging.Logger, err error) {
	configuration = &Configuration{}
	if err = server.ReadConfigurationFile(serverConfigFilename, configuration); err != nil {
		return
	}

	logger, err = configuration.LoggerFactory.NewLogger("caduceus")
	if err != nil {
		return
	}

	return
}

// StartServer is called from main and initializes the server either using OpenStack Swift or a local file system
// and then starts a scheduler that will fetch data at a regular interval from all the WebPA servers
func StartServer(inConfig *Configuration, inLogger logging.Logger) (outMux *mux.Router) {
	serverWrapper := &ServerHandler{
		logger: inLogger,
		workerPool: WorkerPoolFactory{
			NumWorkers: inConfig.NumWorkerThreads,
			QueueSize:  inConfig.JobQueueSize,
		}.New(),
	}

	validator := secure.Validators{
		secure.ExactMatchValidator(inConfig.AuthHeader),
	}

	authHandler := handler.AuthorizationHandler{
		HeaderName:          "Authorization",
		ForbiddenStatusCode: 403,
		Validator:           validator,
		Logger:              inLogger,
	}

	caduceusHandler := alice.New(authHandler.Decorate)

	outMux = mux.NewRouter()
	outMux.Handle("/", caduceusHandler.Then(serverWrapper))

	return
}

func main() {
	var serverConfigFilename string
	flag.StringVar(&serverConfigFilename, "f", DEFAULT_SERVER_CONFIG, "The server config file")
	flag.Parse()

	configuration, logger, err := LoadConfiguration(serverConfigFilename)
	if err != nil {
		fmt.Println("Error loading configuration:", err)
		os.Exit(1)
	}

	logger.Info("Caduceus is up and running!")
	logger.Info("Finished reading config file and generating logger!")
	logger.Info("%v", configuration)

	myMux := StartServer(configuration, logger)

	os.Exit(func() int {
		caduceusHealth := health.New(
			configuration.HealthCheckInterval(),
			logger,
			handler.TotalRequestsReceived,
			handler.TotalRequestSuccessfullyServiced,
			handler.TotalRequestDenied,
		)

		healthServer := (&server.Builder{
			Name:    "caduceus-health",
			Address: configuration.HealthAddress(),
			Logger:  logger,
			Handler: caduceusHealth,
		}).Build()

		caduceusPrimaryServer := (&server.Builder{
			Name:            "caduceus",
			Address:         configuration.PrimaryAddress(),
			CertificateFile: configuration.CertificateFile,
			KeyFile:         configuration.KeyFile,
			Logger:          logger,
			Handler:         myMux,
		}).Build()

		runnables := concurrent.RunnableSet{
			healthServer,
			caduceusPrimaryServer,
		}

		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt)
		err := concurrent.Await(runnables, signals)

		if err != nil {
			logger.Error("Xenos exiting with an error: %v", err)
		} else {
			logger.Info("Received interrupt signal, caduceus exiting...")
		}
		return 0
	}())
}
