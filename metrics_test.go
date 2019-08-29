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

// Using Caduceus's test suite:
//
// If you are testing a new metric the followng process needs to be done below:
// 1. Create a fake, mockMetric i.e fakeEventType := new(mockCounter)
// 2. If your metric type has yet to be included in mockCaduceusMetricRegistry within mocks.go
//    add your metric type to mockCaduceusMetricRegistry
// 3. Trigger the On method on that "mockMetric" with various different cases of that metric,
//    in both senderWrapper_test.go and/or outboundSender_test.go
//    i.e:
//	    case 1: On("With", []string{"event", iot}
//	    case 2: On("With", []string{"event", unknown}
//   Tests for all possible event_types that will be sent to the metrics Desc.  If all cases arn't
//   included tests will fail.
// 4. Mimic the metric behavior using On i.e if your specific metric is a counter:
//      fakeSlow.On("Add", 1.0).Return()

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {
	assert := assert.New(t)

	m := Metrics()

	assert.NotNil(m)
}
