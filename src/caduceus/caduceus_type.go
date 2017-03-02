package main

import (
	"errors"
	"github.com/Comcast/webpa-common/logging"
)

// Below is the struct we're using to contain the data from a provided config file
type CaduceusConfig struct {
	AuthHeader       string
	NumWorkerThreads int
	JobQueueSize     int
}

// Below is the struct we're using to create a request to caduceus
type CaduceusRequest struct {
	Payload     []byte
	ContentType string
	TargetURL   string
}

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	logger     logging.Logger
	workerPool *WorkerPool
}

// Below is the struct and implementation of our worker pool factory
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

// Below is the struct and implementation of our worker pool
// It utilizes a non-blocking channel, so we throw away any requests that exceed
// the channel's limit (indicated by its buffer size)
type WorkerPool struct {
	jobs chan func(workerID int)
}

func (wp *WorkerPool) Send(inFunc func(workerID int)) error {
	select {
	case wp.jobs <- inFunc:
		return nil
	default:
		return errors.New("Worker pool channel full.")
	}
}
