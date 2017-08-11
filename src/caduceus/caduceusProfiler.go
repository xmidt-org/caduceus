package main

import (
	"encoding/json"
	"errors"
	"github.com/Comcast/webpa-common/logging"
	"math"
	"sort"
	"sync"
	"time"
)

type ServerProfilerFactory struct {
	Frequency int
	Duration  int
	QueueSize int
	Parent    ServerProfiler
	Logger    logging.Logger
}

// New will be used to initialize a new server profiler for caduceus and get
// the gears in motion for aggregating data
func (spf ServerProfilerFactory) New(name string) (serverProfiler ServerProfiler, err error) {
	if spf.Frequency < 1 || spf.Duration < 1 || spf.QueueSize < 1 {
		err = errors.New("No parameter to the ServerProfilerFactory can be less than 1.")
		return
	}

	newCaduceusProfiler := &caduceusProfiler{
		name:         name,
		frequency:    spf.Frequency,
		profilerRing: NewCaduceusRing(spf.Duration),
		inChan:       make(chan interface{}, spf.QueueSize),
		quit:         make(chan struct{}),
		rwMutex:      new(sync.RWMutex),
		parent:       spf.Parent,
		logger:       spf.Logger,
	}

	go newCaduceusProfiler.aggregate(newCaduceusProfiler.quit)

	serverProfiler = newCaduceusProfiler
	return
}

type ServerProfiler interface {
	Send(interface{}) error
	Report() []interface{}
	Close()
}

type Tick func(time.Duration) <-chan time.Time

type caduceusProfiler struct {
	name         string
	frequency    int
	tick         Tick
	profilerRing ServerRing
	inChan       chan interface{}
	quit         chan struct{}
	rwMutex      *sync.RWMutex
	parent       ServerProfiler
	logger       logging.Logger
}

// Send will add data that we retrieve onto the
// data structure we use for gathering info
func (cp *caduceusProfiler) Send(inData interface{}) error {
	// send the data over to the structure
	select {
	case cp.inChan <- inData:
		return nil
	default:
		return errors.New("Channel full.")
	}
}

// Report will be used to retrieve data when the data the profiler
// stores is ready to be collected
func (cp *caduceusProfiler) Report() (values []interface{}) {
	cp.rwMutex.RLock()
	values = cp.profilerRing.Snapshot()
	cp.rwMutex.RUnlock()
	return
}

// Close will terminate the running aggregate method and do any cleanup necessary
func (cp *caduceusProfiler) Close() {
	close(cp.quit)
}

// aggregate runs on a timer and will take in data until a certain amount
// of time passes, then it will generate a report that it will share
func (cp *caduceusProfiler) aggregate(quit <-chan struct{}) {
	var data []interface{}
	var ticker <-chan time.Time

	if cp.tick == nil {
		ticker = time.Tick(time.Duration(cp.frequency) * time.Second)
	} else {
		ticker = cp.tick(time.Duration(cp.frequency) * time.Second)
	}

	// Send out a stat at the start of time.
	if nil != cp.parent {
		cp.parent.Send(cp.process(data))
	}

	for {
		select {
		case <-ticker:
			if nil != cp.parent {
				// perform some analysis
				cp.parent.Send(cp.process(data))
			}
			data = nil
		case inData := <-cp.inChan:
			if nil != cp.parent {
				// add the data to a temporary structure
				data = append(data, inData)
			} else {
				// add the data to the ring and clear the temporary structure
				cp.rwMutex.Lock()
				cp.profilerRing.Add(inData)
				cp.rwMutex.Unlock()
			}
		case <-quit:
			return
		}
	}
}

type int64Array []int64

func (a int64Array) Len() int           { return len(a) }
func (a int64Array) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64Array) Less(i, j int) bool { return a[i] < a[j] }

func (cp *caduceusProfiler) process(raw []interface{}) (rv interface{}) {

	raw = filterNonTelemetryElements(raw)
	n := len(raw)

	cs := CaduceusStats{
		Name: cp.name,
		Time: time.Now().String(),
	}

	if 0 < n {
		// in nanoseconds
		latency := make([]int64, n)
		processingTime := make([]int64, n)
		responseTime := make([]int64, n)
		tonnage := 0
		var responseTotal, processingTotal, latencyTotal int64

		for i, rawElement := range raw {
			telemetryData := rawElement.(CaduceusTelemetry)

			tonnage += telemetryData.PayloadSize

			latency[i] = telemetryData.TimeSent.Sub(telemetryData.TimeReceived).Nanoseconds()
			processingTime[i] = telemetryData.TimeOutboundAccepted.Sub(telemetryData.TimeReceived).Nanoseconds()
			responseTime[i] = telemetryData.TimeResponded.Sub(telemetryData.TimeSent).Nanoseconds()

			latencyTotal += latency[i]
			processingTotal += processingTime[i]
			responseTotal += responseTime[i]
		}
		sort.Sort(int64Array(latency))
		sort.Sort(int64Array(processingTime))
		sort.Sort(int64Array(responseTime))

		// TODO There is a pattern for time based stats calculations that should be made common

		// get98th returns the 98% indice value.
		// example: in an array with length of 100. index 97 would be the 98th.
		get98th := func(list []int64) int64 {
			return int64(math.Ceil(float64(len(list))*0.98) - 1)
		}

		cs.Tonnage = tonnage
		cs.EventsSent = n
		cs.ProcessingTimePerc98 = time.Duration(processingTime[get98th(processingTime)]).String()
		cs.ProcessingTimeAvg = time.Duration(processingTotal / int64(n)).String()
		cs.LatencyPerc98 = time.Duration(latency[get98th(latency)]).String()
		cs.LatencyAvg = time.Duration(latencyTotal / int64(n)).String()
		cs.ResponsePerc98 = time.Duration(responseTime[get98th(responseTime)]).String()
		cs.ResponseAvg = time.Duration(responseTotal / int64(n)).String()
	}

	rv = &cs

	b, err := json.Marshal(cs)
	if nil == err {
		cp.logger.Error("Endpoint Delivery Stats: %s", string(b))
	} else {
		cp.logger.Error("Endpoint Delivery Stats: %+v", cs)
	}

	return
}

//Input: An array A of interfaces
//Output: An array A' containing those elements in A that cast to type CaduceusTelemetry
func filterNonTelemetryElements(elements []interface{}) (output []interface{}) {
	for _, element := range elements {
		if _, isCaduceusTelemetry := element.(CaduceusTelemetry); isCaduceusTelemetry {
			output = append(output, element)
		}
	}
	return
}
