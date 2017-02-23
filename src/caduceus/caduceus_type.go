package main

import (
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/logging/golog"
	"github.com/Comcast/webpa-common/server"
)

// Below is the struct we're using to contain the data from a provided config file
type Configuration struct {
	AuthHeader       string              `json:"auth_header"`
	LoggerFactory    golog.LoggerFactory `json:"log"`
	NumWorkerThreads int                 `json:"num_workers"`
	JobQueueSize     int                 `json:"queue_size"`
	server.Configuration
}

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	logger     logging.Logger
	workerPool *WorkerPool
}

type WorkerPoolFactory struct {
	NumWorkers int
	QueueSize  int
}

func (wpf WorkerPoolFactory) New() (wp *WorkerPool) {
	jobs := make(chan func(workerID int), wpf.QueueSize)

	for i := 0; i < wpf.NumWorkers; i++ {
		go func(id int) {
			for f := range jobs {
				f(id)
			}
		}(i)
	}

	wp = &WorkerPool{
		jobs: jobs,
	}

	return
}

type WorkerPool struct {
	jobs chan func(workerID int)
}

func (wp *WorkerPool) Send(inFunc func(workerID int)) {
	wp.jobs <- inFunc
}
