package main

import (
	"errors"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
)

const (
	// Stuff we're looking at health-wise
	// TODO: figure out how to add per IP buckets
	PayloadsOverZero        health.Stat = "PayloadsOverZero"
	PayloadsOverHundred     health.Stat = "PayloadsOverHundred"
	PayloadsOverThousand    health.Stat = "PayloadsOverThousand"
	PayloadsOverTenThousand health.Stat = "PayloadsOverTenThousand"
	TotalMessagesAccepted   health.Stat = "TotalMessagesAccepted"
	TotalMessagesDropped    health.Stat = "TotalMessagesDropped"
)

// Below is the struct we're using to contain the data from a provided config file
// TODO: Try to figure out how to make bucket ranges configurable
type CaduceusConfig struct {
	AuthHeader                          string
	NumWorkerThreads                    int
	JobQueueSize                        int
	TotalIncomingPayloadSizeBuckets     []int
	PerSourceIncomingPayloadSizeBuckets []int
}

// Below is the struct we're using to create a request to caduceus
type CaduceusRequest struct {
	Payload     []byte
	ContentType string
	TargetURL   string
}

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	logger        logging.Logger
	workerPool    *WorkerPool
	healthTracker HealthTracker
}

// Below is the struct and implementation of how we're tracking health stuff
type HealthTracker struct {
	healthMonitor health.Monitor
}

func (ht *HealthTracker) Increment(inStat health.Stat) {
	ht.healthMonitor.SendEvent(health.Inc(inStat, 1))
}

func (ht *HealthTracker) IncrementBucket(inSize int) {
	if inSize < 100 {
		ht.healthMonitor.SendEvent(health.Inc(PayloadsOverZero, 1))
	} else if inSize < 1000 {
		ht.healthMonitor.SendEvent(health.Inc(PayloadsOverHundred, 1))
	} else if inSize < 1000 {
		ht.healthMonitor.SendEvent(health.Inc(PayloadsOverThousand, 1))
	} else {
		ht.healthMonitor.SendEvent(health.Inc(PayloadsOverTenThousand, 1))
	}
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
