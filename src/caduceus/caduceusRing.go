package main

import (
	"container/ring"
)

// NewCaduceusRing is used to instantiate a new ring for profiling outbound messages
func NewCaduceusRing(inSize int) (myServerRing ServerRing) {
	myServerRing = &caduceusRing{
		Ring: ring.New(inSize),
	}

	return
}

// methods that we allow the user of a caduceusRing to access
type ServerRing interface {
	Add(inValue interface{})
	Snapshot() (values []interface{})
}

// underlying implementation of the ring
type caduceusRing struct {
	*ring.Ring
}

// Add adds a value to the server ring, which will overwrite the oldest value if full
func (sr *caduceusRing) Add(inValue interface{}) {
	sr.Value = inValue
	sr.Ring = sr.Next()
}

// Snapshot is used to grab the ring and store it as an array so we can profile some things
func (sr *caduceusRing) Snapshot() (values []interface{}) {
	sr.Do(func(x interface{}) {
		if x != nil {
			values = append(values, x)
		}
	})

	return
}
