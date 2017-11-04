/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
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
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCaduceusRing(t *testing.T) {
	assert := assert.New(t)

	testCaduceusRing := NewCaduceusRing(10)

	assert.NotNil(testCaduceusRing)
}

func TestCaduceusRing(t *testing.T) {
	assert := assert.New(t)

	testCaduceusRing := NewCaduceusRing(10)
	testCaduceusRing.Add("test1")
	testCaduceusRing.Add("test2")

	t.Run("TestCaduceusRingSnapshot", func(t *testing.T) {
		testValues := testCaduceusRing.Snapshot()
		assert.Equal(2, len(testValues))
		assert.Equal("test1", testValues[0].(string))
		assert.Equal("test2", testValues[1].(string))
	})
}
