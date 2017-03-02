package main

import (
	"bytes"
	"fmt"
	"net/http"
)

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	sh.logger.Info("Someone is saying hello!")
	fmt.Fprintf(response, "%s", []byte("Heyo whaddup!\n"))

	myBuf := new(bytes.Buffer)
	myBuf.ReadFrom(request.Body)
	myPayload := myBuf.Bytes()

	sh.workerPool.Send(func(workerID int) { sh.HandleRequest(workerID, myPayload) })
	request.Body.Close()
}

func (sh *ServerHandler) HandleRequest(workerID int, inPayload []byte) {
	sh.logger.Info("Worker ID #%d is handling a request...", workerID)

	sh.logger.Info("Worker ID #%d is printing the payload: %s", workerID, string(inPayload))
}
