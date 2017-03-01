package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()

	sh.logger.Info("Someone is saying hello!")
	fmt.Fprintf(response, "%s", []byte("Heyo whaddup!\n"))

	var myPayload JSONPayload
	err := json.NewDecoder(request.Body).Decode(&myPayload)
	if err != nil {
		sh.logger.Info("Failed to parse the payload: %s", err.Error())
		fmt.Fprintf(response, "%s", []byte("Payload failed to parse!\n"))
	}

	sh.workerPool.Send(func(workerID int) { sh.HandleRequest(workerID, myPayload) })
}

func (sh *ServerHandler) HandleRequest(workerID int, inPayload JSONPayload) {
	sh.logger.Info("Worker ID #%d is printing the payload: %s", workerID, inPayload.Payload)
}
