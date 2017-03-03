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
	TimeReceived        int64
	TimeAccepted        int64
	TimeProcessingStart int64
	TimeProcessingEnd   int64
}

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	logger          logging.Logger
	caduceusHandler RequestHandler
	caduceusHealth  HealthTracker
	workerPool      *WorkerPool
}

type RequestHandler interface {
	HandleRequest(workerID int, inRequest CaduceusRequest)
}

type CaduceusHandler struct {
	logger logging.Logger
}

func (ch *CaduceusHandler) HandleRequest(workerID int, inRequest CaduceusRequest) {
	inRequest.Timestamps.TimeProcessingStart = time.Now().UnixNano()

	ch.logger.Info("Worker #%d received a request, payload:\t%s", workerID, string(inRequest.Payload))
	ch.logger.Info("Worker #%d received a request, type:\t\t%s", workerID, inRequest.ContentType)
	ch.logger.Info("Worker #%d received a request, url:\t\t%s", workerID, inRequest.TargetURL)

	inRequest.Timestamps.TimeProcessingEnd = time.Now().UnixNano()

	ch.logger.Info("Worker #%d printing elapsed message time:\t%v", workerID, inRequest.Timestamps)
}

type HealthTracker interface {
	Increment(inStat health.Stat)
	IncrementBucket(inSize int)
}

// Below is the struct and implementation of how we're tracking health stuff
type CaduceusHealth struct {
	healthMonitor health.Monitor
}

func (ch *CaduceusHealth) Increment(inStat health.Stat) {
	ch.healthMonitor.SendEvent(health.Inc(inStat, 1))
}

func (ch *CaduceusHealth) IncrementBucket(inSize int) {
	if inSize < 100 {
		ch.healthMonitor.SendEvent(health.Inc(PayloadsOverZero, 1))
	} else if inSize < 1000 {
		ch.healthMonitor.SendEvent(health.Inc(PayloadsOverHundred, 1))
	} else if inSize < 1000 {
		ch.healthMonitor.SendEvent(health.Inc(PayloadsOverThousand, 1))
	} else {
		ch.healthMonitor.SendEvent(health.Inc(PayloadsOverTenThousand, 1))
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
