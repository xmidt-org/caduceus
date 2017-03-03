package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()

	sh.logger.Info("Someone is saying hello!")
	fmt.Fprintf(response, "%s", []byte("Heyo whaddup!\n"))

	timeStamps := CaduceusTimestamps{
		TimeReceived: time.Now().UnixNano(),
	}

	myPayload, err := ioutil.ReadAll(request.Body)
	if err != nil {
		statusMsg := "Unable to retrieve the request body: " + err.Error() + ".\n"
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte(statusMsg))
		return
	}

	var contentType string
	if value, ok := request.Header["Content-Type"]; ok {
		if len(value) == 1 {
			contentType = value[0]
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
		Payload:     myPayload,
		ContentType: contentType,
		TargetURL:   targetURL,
		Timestamps:  timeStamps,
	}

	caduceusRequest.Timestamps.TimeAccepted = time.Now().UnixNano()

	err = sh.workerPool.Send(func(workerID int) { sh.HandleRequest(workerID, caduceusRequest) })
	if err != nil {
		// return a 408
		response.WriteHeader(http.StatusRequestTimeout)
		response.Write([]byte("Unable to handle request at this time.\n"))
		sh.healthTracker.Increment(TotalMessagesDropped)
	} else {
		// return a 202
		response.WriteHeader(http.StatusAccepted)
		response.Write([]byte("Request placed on to queue.\n"))
		sh.healthTracker.Increment(TotalMessagesAccepted)
		sh.healthTracker.IncrementBucket(len(myPayload))
	}
}

func (sh *ServerHandler) HandleRequest(workerID int, inRequest CaduceusRequest) {
	inRequest.Timestamps.TimeProcessingStart = time.Now().UnixNano()

	sh.logger.Info("Worker #%d received a request, payload:\t%s", workerID, string(inRequest.Payload))
	sh.logger.Info("Worker #%d received a request, type:\t\t%s", workerID, inRequest.ContentType)
	sh.logger.Info("Worker #%d received a request, url:\t\t%s", workerID, inRequest.TargetURL)

	inRequest.Timestamps.TimeProcessingEnd = time.Now().UnixNano()

	sh.logger.Info("Worker #%d printing elapsed message time:\t%v", workerID, inRequest.Timestamps)
}
