package main

import (
	"fmt"
	"net/http"
)

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	sh.logger.Info("Someone is saying hello!")
	fmt.Fprintf(response, "%s", []byte("Heyo whaddup!\n"))

	for i := 0; i < 100; i++ {
		sh.workerPool.Send(func(workerID int) { fmt.Println("Worker #", workerID, "says hi!") })
	}
}
