package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()

	sh.logger.Info("Someone is saying hello!")
	fmt.Fprintf(response, "%s", []byte("Heyo whaddup!\n"))

	myPayload, err := ioutil.ReadAll(request.Body)
	if err != nil {
		fmt.Fprintf(response, "Unable to retrieve the request body: %s", err.Error())
		return
	}

	var contentType string
	if value, ok := request.Header["Content-Type"]; ok {
		if len(value) == 1 {
			contentType = value[0]
		} else {
			fmt.Fprintf(response, "There cannot be more than one content type in the request header!")
		}
	} else {
		fmt.Fprintf(response, "Content-Type must be set in the header!")
	}

	if contentType == "" {
		return
	}

	targetURL := request.URL.String()

	caduceusRequest := CaduceusRequest{
		Payload:     myPayload,
		ContentType: contentType,
		TargetURL:   targetURL,
	}

	sh.workerPool.Send(func(workerID int) { sh.HandleRequest(workerID, caduceusRequest) })
}

func (sh *ServerHandler) HandleRequest(workerID int, inRequest CaduceusRequest) {
	sh.logger.Info("Worker #%d received a request, payload:\t%s", workerID, string(inRequest.Payload))
	sh.logger.Info("Worker #%d received a request, type:\t\t%s", workerID, inRequest.ContentType)
	sh.logger.Info("Worker #%d received a request, url:\t\t%s", workerID, inRequest.TargetURL)
}
