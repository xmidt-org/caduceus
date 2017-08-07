package main

import (
	"errors"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/secure"
	"github.com/Comcast/webpa-common/secure/key"
	"net/http"
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
	AuthHeader                          []string
	NumWorkerThreads                    int
	JobQueueSize                        int
	SenderNumWorkersPerSender           int
	SenderQueueSizePerSender            int
	SenderCutOffPeriod                  int
	SenderLinger                        int
	SenderClientTimeout                 int
	ProfilerFrequency                   int
	ProfilerDuration                    int
	ProfilerQueueSize                   int
	TotalIncomingPayloadSizeBuckets     []int
	PerSourceIncomingPayloadSizeBuckets []int
	JWTValidators                       []JWTValidator
}

type JWTValidator struct {
	// JWTKeys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory

	// Custom is an optional configuration section that defines
	// custom rules for validation over and above the standard RFC rules.
	Custom secure.JWTValidatorFactory
}

// Below is the struct we're using to create a request to caduceus
type CaduceusRequest struct {
	Payload     []byte
	ContentType string
	TargetURL   string
	Telemetry   CaduceusTelemetry
}

type CaduceusTelemetry struct {
	PayloadSize          int
	TimeReceived         time.Time
	TimeAccepted         time.Time
	TimeSentToOutbound   time.Time
	TimeOutboundAccepted time.Time
	TimeSent             time.Time
	TimeResponded        time.Time
}

type CaduceusStats struct {
	Name                 string `json:"endpoint-name"`
	Time                 string `json:"time"`
	Tonnage              int    `json:"tonnage"`
	EventsSent           int    `json:"events-sent"`
	ProcessingTimePerc98 string `json:"processing-time-perc98"`
	ProcessingTimeAvg    string `json:"processing-time-avg"`
	LatencyPerc98        string `json:"latency-perc98"`
	LatencyAvg           string `json:"latency-avg"`
	ResponsePerc98       string `json:"response-perc98"`
	ResponseAvg          string `json:"response-avg"`
}

type RequestHandler interface {
	HandleRequest(workerID int, inRequest CaduceusRequest)
}

type CaduceusHandler struct {
	handlerProfiler ServerProfiler
	senderWrapper   SenderWrapper
	logging.Logger
}

func (ch *CaduceusHandler) HandleRequest(workerID int, inRequest CaduceusRequest) {
	inRequest.Telemetry.TimeSentToOutbound = time.Now()

	ch.Info("Worker #%d received a request, now passing on to sender wrapper...", workerID)
	ch.senderWrapper.Queue(inRequest)
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

func (ch *CaduceusHealth) dnsReady(address string, ready chan bool) {	
	req, err := http.NewRequest("GET", address, nil)
	if err != nil {
		return
	}
	client := http.Client{}

	var resp *http.Response
	for resp == nil {
		resp, err = client.Do(req)
		if err == nil {
			ready <- true
			break
		}
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
