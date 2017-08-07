package main

import (
	"encoding/json"
	"fmt"
	"github.com/Comcast/webpa-common/logging"
	"io/ioutil"
	"net/http"
	"time"
)

type Send func(inFunc func(workerID int)) error

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	logging.Logger
	caduceusHandler RequestHandler
	caduceusHealth  HealthTracker
	doJob           Send
}

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()

	sh.Info("Receiving incoming request...")

	stats := CaduceusTelemetry{
		TimeReceived: time.Now(),
	}

	if request.Method != "POST" {
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte(fmt.Sprintf("Unsupported method \"%s\"... Caduceus only supports \"POST\" method.\n", request.Method)))
		return
	}

	payload, err := ioutil.ReadAll(request.Body)
	if err != nil {
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte(fmt.Sprintf("Unable to retrieve the request body: %s.\n", err.Error)))
		return
	}

	var contentType string
	if value, ok := request.Header["Content-Type"]; ok {
		if len(value) == 1 {
			contentType = value[0]
			switch contentType {
			case "application/json":
				// ok contentType
			case "application/wrp":
				// ok contentType
			default:
				response.WriteHeader(http.StatusBadRequest)
				response.Write([]byte("Only Content-Type values of \"application/json\" or \"application/wrp\" are supported.\n"))
				return
			}
		} else {
			response.WriteHeader(http.StatusBadRequest)
			response.Write([]byte("Content-Type cannot have more than one specification.\n"))
		}
	} else {
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte("Content-Type must be set in the header.\n"))
	}

	if contentType == "" {
		return
	}

	targetURL := request.URL.String()

	caduceusRequest := CaduceusRequest{
		Payload:     payload,
		ContentType: contentType,
		TargetURL:   targetURL,
		Telemetry:   stats,
	}

	caduceusRequest.Telemetry.PayloadSize = len(payload)
	caduceusRequest.Telemetry.TimeAccepted = time.Now()

	err = sh.doJob(func(workerID int) { sh.caduceusHandler.HandleRequest(workerID, caduceusRequest) })
	if err != nil {
		// return a 408
		response.WriteHeader(http.StatusRequestTimeout)
		response.Write([]byte("Unable to handle request at this time.\n"))
	} else {
		// return a 202
		response.WriteHeader(http.StatusAccepted)
		response.Write([]byte("Request placed on to queue.\n"))
		sh.caduceusHealth.IncrementBucket(caduceusRequest.Telemetry.PayloadSize)
	}
}

type ProfileHandler struct {
	profilerData ServerProfiler
	logging.Logger
}

// ServeHTTP method of ProfileHandler will output the most recent messages
// that the main handler has successfully dealt with
func (ph *ProfileHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()

	ph.Info("Receiving request for server stats...")
	stats := ph.profilerData.Report()
	b, err := json.Marshal(stats)

	if nil == stats {
		b = []byte("[]")
		err = nil
	}

	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte("Error marshalling the data into a JSON object."))
	} else {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		response.Write(b)
		response.Write([]byte("\n"))
	}
}
