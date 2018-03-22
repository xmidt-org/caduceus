/**
 * Copyright 2018 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleCounter(t *testing.T) {
	assert := assert.New(t)

	s := &SimpleCounter{Count: 0}
	newS := s.With("foo", "bar")
	assert.True(s == newS)

	s.Add(1.0)
	assert.True(1.0 == s.Count)

	s.Add(10.0)
	assert.True(11.0 == s.Count)

	s.Add(-10.0)
	assert.True(11.0 == s.Count)
}
