package main

import (
	"errors"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
	"time"
)

const (
	// Stuff we're looking at health-wise
	// TODO: figure out how to add per IP buckets
	PayloadsOverZero        health.Stat = "PayloadsOverZero"
	PayloadsOverHundred     health.Stat = "PayloadsOverHundred"
	PayloadsOverThousand    health.Stat = "PayloadsOverThousand"
	PayloadsOverTenThousand health.Stat = "PayloadsOverTenThousand"
)

// Below is the struct we're using to contain the data from a provided config file
// TODO: Try to figure out how to make bucket ranges configurable
type CaduceusConfig struct {
	AuthHeader                          string
	NumWorkerThreads                    int
	JobQueueSize                        int
	ProfilerFrequency                   int
	ProfilerDuration                    int
	ProfilerQueueSize                   int
	TotalIncomingPayloadSizeBuckets     []int
	PerSourceIncomingPayloadSizeBuckets []int
}

// Below is the struct we're using to create a request to caduceus
type CaduceusRequest struct {
	Payload     []byte
	ContentType string
	TargetURL   string
	Timestamps  CaduceusTimestamps
}

type CaduceusTimestamps struct {
	TimeReceived        time.Time
	TimeAccepted        time.Time
	TimeProcessingStart time.Time
	TimeProcessingEnd   time.Time
}

type RequestHandler interface {
	HandleRequest(workerID int, inRequest CaduceusRequest)
}

type CaduceusHandler struct {
	logging.Logger
	caduceusProfiler ServerProfiler
}

func (ch *CaduceusHandler) HandleRequest(workerID int, inRequest CaduceusRequest) {
	inRequest.Timestamps.TimeProcessingStart = time.Now()

	ch.Info("Worker #%d received a request, payload:\t%s", workerID, string(inRequest.Payload))
	ch.Info("Worker #%d received a request, type:\t\t%s", workerID, inRequest.ContentType)
	ch.Info("Worker #%d received a request, url:\t\t%s", workerID, inRequest.TargetURL)

	inRequest.Timestamps.TimeProcessingEnd = time.Now()

	// TODO: temporarily adding the whole request to a profiler for testing purposes
	ch.caduceusProfiler.Send(inRequest)

	ch.Info("Worker #%d printing message time stats:\t%v", workerID, inRequest.Timestamps)
}

type HealthTracker interface {
	SendEvent(health.HealthFunc)
	IncrementBucket(inSize int)
}

// Below is the struct and implementation of how we're tracking health stuff
type CaduceusHealth struct {
	health.Monitor
}

func (ch *CaduceusHealth) IncrementBucket(inSize int) {
	if inSize < 101 {
		ch.SendEvent(health.Inc(PayloadsOverZero, 1))
	} else if inSize < 1001 {
		ch.SendEvent(health.Inc(PayloadsOverHundred, 1))
	} else if inSize < 10001 {
		ch.SendEvent(health.Inc(PayloadsOverThousand, 1))
	} else {
		ch.SendEvent(health.Inc(PayloadsOverTenThousand, 1))
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
