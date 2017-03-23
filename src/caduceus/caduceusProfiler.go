package main

import (
	"errors"
	"sync"
	"time"
)

type ServerProfilerFactory struct {
	Frequency int
	Duration  int
	QueueSize int
}

// New will be used to initialize a new server profiler for caduceus and get
// the gears in motion for aggregating data
func (spf ServerProfilerFactory) New() (serverProfiler ServerProfiler) {
	newCaduceusProfiler := &caduceusProfiler{
		frequency:    spf.Frequency,
		profilerRing: NewCaduceusRing(spf.Duration),
		inChan:       make(chan interface{}, spf.QueueSize),
		quit:         make(chan struct{}),
		rwMutex:      &sync.RWMutex{},
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
	frequency    int
	tick         Tick
	profilerRing ServerRing
	inChan       chan interface{}
	quit         chan struct{}
	rwMutex      *sync.RWMutex
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

	for {
		select {
		case <-ticker:
			// add the data to the ring and clear the temporary structure
			cp.rwMutex.Lock()
			cp.profilerRing.Add(data)
			cp.rwMutex.Unlock()
			data = nil
		case inData := <-cp.inChan:
			// add the data to a temporary structure
			data = append(data, inData)
		case <-quit:
			return
		}
	}
}
