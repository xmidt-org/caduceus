/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */
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
