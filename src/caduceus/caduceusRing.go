package main

import (
	"container/ring"
	"sync"
)

// NewCaduceusRing is used to instantiate a new ring for profiling outbound messages
func NewCaduceusRing(inSize int) (myServerRing ServerRing) {
	myServerRing = &caduceusRing{
		Ring:    ring.New(inSize),
		rwMutex: new(sync.RWMutex),
	}

	return
}

// methods that we allow the user of a caduceusRing to access
type ServerRing interface {
	Add(interface{})
	Snapshot() []interface{}
}

// underlying implementation of the ring
type caduceusRing struct {
	*ring.Ring
	rwMutex *sync.RWMutex
}

// Add adds a value to the server ring, which will overwrite the oldest value if full
func (sr *caduceusRing) Add(inValue interface{}) {
	sr.rwMutex.Lock()
	sr.Value = inValue
	sr.Ring = sr.Next()
	sr.rwMutex.Unlock()
}

// Snapshot is used to grab the ring and store it as an array so we can profile some things
func (sr *caduceusRing) Snapshot() (values []interface{}) {
	sr.rwMutex.RLock()
	sr.Do(func(x interface{}) {
		if x != nil {
			values = append(values, x)
		}
	})
	sr.rwMutex.RUnlock()

	return
}
