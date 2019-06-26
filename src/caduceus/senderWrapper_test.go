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
	"bytes"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/Comcast/wrp-go/wrp"
	"github.com/stretchr/testify/assert"
)

type result struct {
	URL      string
	event    string
	transID  string
	deviceID string
}

// Make a simple RoundTrip implementation that let's me short-circuit the network
type swTransport struct {
	i       int32
	fn      func(*http.Request, int) (*http.Response, error)
	results []result
	mutex   sync.Mutex
}

func (t *swTransport) RoundTrip(req *http.Request) (*http.Response, error) {

	//
	atomic.AddInt32(&t.i, 1)

	r := result{URL: req.URL.String(),
		event:    req.Header.Get("X-Webpa-Event"),
		transID:  req.Header.Get("X-Webpa-Transaction-Id"),
		deviceID: req.Header.Get("X-Webpa-Device-Id"),
	}

	t.mutex.Lock()
	t.results = append(t.results, r)
	t.mutex.Unlock()

	resp := &http.Response{Status: "200 OK", StatusCode: 200}
	return resp, nil
}

func getFakeFactory() *SenderWrapperFactory {

	fakeDDTIP := new(mockCounter)
	fakeDDTIP.On("Add", 1.0).Return()

	fakeGauge := new(mockGauge)
	fakeGauge.On("Add", 1.0).Return().
		On("Add", -1.0).Return().
		//On("With", []string{"url", "unknown"}).Return(fakeGauge).
		On("With", []string{"url", "http://localhost:8888/foo"}).Return(fakeGauge).
		On("With", []string{"url", "http://localhost:9999/foo"}).Return(fakeGauge)

	fakeIgnore := new(mockCounter)
	fakeIgnore.On("Add", 1.0).Return().On("Add", 0.0).Return().
		On("With", []string{"url", "http://localhost:8888/foo"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:8888/foo", "event", "unknown"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo", "event", "unknown"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:8888/foo", "reason", "cut_off"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:8888/foo", "reason", "queue_full"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:8888/foo", "reason", "expired"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:8888/foo", "reason", "expired_before_queueing"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:8888/foo", "reason", "network_err"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:8888/foo", "reason", "invalid_config"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo", "reason", "cut_off"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo", "reason", "queue_full"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo", "reason", "expired"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo", "reason", "expired_before_queueing"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo", "reason", "network_err"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo", "reason", "invalid_config"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:8888/foo", "code", "200", "event", "unknown"}).Return(fakeIgnore).
		On("With", []string{"url", "http://localhost:9999/foo", "code", "200", "event", "unknown"}).Return(fakeIgnore).
		On("With", []string{"event", "iot"}).Return(fakeIgnore).
		On("With", []string{"event", "test/extra-stuff"}).Return(fakeIgnore).
		On("With", []string{"event", "bob/magic/dog"}).Return(fakeIgnore).
		On("With", []string{"event", "unknown"}).Return(fakeIgnore).
		On("With", []string{"content_type", "msgpack"}).Return(fakeIgnore).
		On("With", []string{"content_type", "json"}).Return(fakeIgnore).
		On("With", []string{"content_type", "http"}).Return(fakeIgnore).
		On("With", []string{"content_type", "other"}).Return(fakeIgnore)

	fakeRegistry := new(mockCaduceusMetricsRegistry)
	fakeRegistry.On("NewCounter", DropsDueToInvalidPayload).Return(fakeDDTIP)
	fakeRegistry.On("NewCounter", DeliveryRetryCounter).Return(fakeIgnore)
	fakeRegistry.On("NewCounter", DeliveryCounter).Return(fakeIgnore)
	fakeRegistry.On("NewCounter", SlowConsumerCounter).Return(fakeIgnore)
	fakeRegistry.On("NewCounter", SlowConsumerDroppedMsgCounter).Return(fakeIgnore)
	fakeRegistry.On("NewCounter", IncomingContentTypeCounter).Return(fakeIgnore)
	fakeRegistry.On("NewCounter", IncomingEventTypeCounter).Return(fakeIgnore)
	fakeRegistry.On("NewGauge", OutgoingQueueDepth).Return(fakeGauge)
	fakeRegistry.On("NewGauge", DeliveryRetryMaxGauge).Return(fakeGauge)
	fakeRegistry.On("NewGauge", ConsumerRenewalTimeGauge).Return(fakeGauge)
	fakeRegistry.On("NewGauge", ConsumerDeliverUntilGauge).Return(fakeGauge)
	fakeRegistry.On("NewGauge", ConsumerDropUntilGauge).Return(fakeGauge)
	fakeRegistry.On("NewGauge", ConsumerDeliveryWorkersGauge).Return(fakeGauge)
	fakeRegistry.On("NewGauge", ConsumerMaxDeliveryWorkersGauge).Return(fakeGauge)

	return &SenderWrapperFactory{
		NumWorkersPerSender: 10,
		QueueSizePerSender:  10,
		CutOffPeriod:        30 * time.Second,
		Logger:              logging.DefaultLogger(),
		Linger:              0 * time.Second,
		MetricsRegistry:     fakeRegistry,
	}
}

func TestInvalidLinger(t *testing.T) {
	swf := getFakeFactory()
	sw, err := swf.New()

	assert := assert.New(t)
	assert.Nil(sw)
	assert.NotNil(err)
}

// Commenting this test out is accumulating technical debt.
// The reason this code doesn't work now is because the timeout in webpa-common
// is hard coded to 5min at this point.  The ways to address this are:
// 1. Remove the limitation of 5min as the only timeout
// -or-
// 2. Add a mock for the webhook implementation

func TestSwSimple(t *testing.T) {
	assert := assert.New(t)

	wrpMessage := wrp.SimpleRequestResponse{
		Source:          "mac:112233445566",
		Destination:     "event:wrp",
		TransactionUUID: "12345",
	}

	var buffer bytes.Buffer
	encoder := wrp.NewEncoder(&buffer, wrp.Msgpack)
	err := encoder.Encode(&wrpMessage)
	assert.Nil(err)

	iot := simpleRequest()
	iot.Destination = "mac:112233445566/event/iot"
	test := simpleRequest()
	test.Destination = "mac:112233445566/event/test/extra-stuff"

	trans := &swTransport{}

	swf := getFakeFactory()
	swf.Sender = trans.RoundTrip

	swf.Linger = 1 * time.Second
	sw, err := swf.New()

	assert.Nil(err)
	assert.NotNil(sw)

	// No listeners
	sw.Queue(iot)
	sw.Queue(iot)
	sw.Queue(iot)

	assert.Equal(int32(0), trans.i)

	w1 := webhook.W{
		Duration: 6 * time.Second,
		Until:    time.Now().Add(6 * time.Second),
		Events:   []string{"iot"},
	}
	w1.Config.URL = "http://localhost:8888/foo"
	w1.Config.ContentType = "application/json"
	w1.Matcher.DeviceId = []string{"mac:112233445566"}

	w2 := webhook.W{
		Duration: 4 * time.Second,
		Until:    time.Now().Add(4 * time.Second),
		Events:   []string{"iot", "test/extra-stuff", "wrp"},
	}
	w2.Config.URL = "http://localhost:9999/foo"
	w2.Config.ContentType = "application/json"
	w2.Matcher.DeviceId = []string{"mac:112233445566"}

	// Add 2 listeners
	list := []webhook.W{w1, w2}

	sw.Update(list)

	// Send iot message

	sw.Queue(iot)

	// Send test message
	sw.Queue(test)

	// Send it again
	sw.Queue(test)

	w3 := webhook.W{
		Events: []string{"iot"},
	}
	w3.Config.URL = "http://localhost:9999/foo"
	w3.Config.ContentType = "application/json"

	// We get a registration
	list2 := []webhook.W{w3}
	sw.Update(list2)
	time.Sleep(time.Second)

	// Send iot
	sw.Queue(iot)

	sw.Shutdown(true)
	//assert.Equal(int32(4), atomic.LoadInt32(&trans.i))
}
