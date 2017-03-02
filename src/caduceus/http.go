package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	sh.logger.Info("Someone is saying hello!")
	fmt.Fprintf(response, "%s", []byte("Heyo whaddup!\n"))

	myPayload, err := ioutil.ReadAll(request.Body)
	if err != nil {
		fmt.Fprintf(response, "%s", []byte("Unable to retrieve request body!\n"))
	} else {
		sh.workerPool.Send(func(workerID int) { sh.HandleRequest(workerID, myPayload) })
	}

	request.Body.Close()
}

func (sh *ServerHandler) HandleRequest(workerID int, inPayload []byte) {
	sh.logger.Info("Worker ID #%d is handling a request...", workerID)

	sh.logger.Info("Worker ID #%d is printing the payload: %s", workerID, string(inPayload))
}
