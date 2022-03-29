/**
 * Copyright 2020 Comcast Cable Communications Management, LLC
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
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/wrp-go/v3"

	//"github.com/stretchr/testify/mock"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// Make a simple RoundTrip implementation that let's me short-circuit the network
type transport struct {
	i  int32
	fn func(*http.Request, int) (*http.Response, error)
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	i := atomic.AddInt32(&t.i, 1)
	return t.fn(req, int(i))
}

func getNewTestOutputLogger(out io.Writer) log.Logger {
	return log.NewLogfmtLogger(out)
}

func simpleSetup(trans *transport, cutOffPeriod time.Duration, matcher []string) (OutboundSender, error) {
	return simpleFactorySetup(trans, cutOffPeriod, matcher).New()
}

// simpleFactorySetup sets up a outboundSender with metrics.
//
// Using Caduceus's test suite
//
// If you are testing a new metric it needs to be created in this process below.
// 1. Create a fake, mockMetric i.e fakeEventType := new(mockCounter)
// 2. If your metric type has yet to be included in mockCaduceusMetricRegistry within mocks.go
//    add your metric type to mockCaduceusMetricRegistry
// 3. Trigger the On method on that "mockMetric" with various different cases of that metric,
//    in both senderWrapper_test.go and outboundSender_test.go
//    i.e:
//	    case 1: On("With", []string{"event", iot}
//	    case 2: On("With", []string{"event", unknown}
// 4. Mimic the metric behavior using On:
//      fakeSlow.On("Add", 1.0).Return()
func simpleFactorySetup(trans *transport, cutOffPeriod time.Duration, matcher []string) *OutboundSenderFactory {
	if nil == trans.fn {
		trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
			resp = &http.Response{Status: "200 OK",
				StatusCode: 200,
			}
			return
		}
	}

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now().Add(60 * time.Second),
			Events: []string{"iot", "test"},
			Config: ancla.DeliveryConfig{
				URL:         "http://localhost:9999/foo",
				ContentType: wrp.MimeTypeJson,
				Secret:      "123456",
			},
		},
		PartnerIDs: []string{"comcast"},
	}
	w.Webhook.Matcher.DeviceID = matcher

	// test dc metric
	fakeDC := new(mockCounter)
	fakeDC.On("With", []string{"url", w.Webhook.Config.URL, "code", "200", "event", "test"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "code", "200", "event", "iot"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "code", "200", "event", "unknown"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "code", "failure", "event", "iot"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "event", "test"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "event", "iot"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "event", "unknown"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "code", "201"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "code", "202"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "code", "204"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "code", "429", "event", "iot"}).Return(fakeDC).
		On("With", []string{"url", w.Webhook.Config.URL, "code", "failure"}).Return(fakeDC)
	fakeDC.On("Add", 1.0).Return()
	fakeDC.On("Add", 0.0).Return()

	// test slow metric
	fakeSlow := new(mockCounter)
	fakeSlow.On("With", []string{"url", w.Webhook.Config.URL}).Return(fakeSlow)
	fakeSlow.On("Add", 1.0).Return()

	// test dropped metric
	fakeDroppedSlow := new(mockCounter)
	fakeDroppedSlow.On("With", []string{"url", w.Webhook.Config.URL, "reason", "queue_full"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", []string{"url", w.Webhook.Config.URL, "reason", "cut_off"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", []string{"url", w.Webhook.Config.URL, "reason", "expired"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", []string{"url", w.Webhook.Config.URL, "reason", "expired_before_queueing"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", []string{"url", w.Webhook.Config.URL, "reason", "invalid_config"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", []string{"url", w.Webhook.Config.URL, "reason", "network_err"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("Add", mock.Anything).Return()

	// IncomingContentType cases
	fakeContentType := new(mockCounter)
	fakeContentType.On("With", []string{"content_type", "msgpack"}).Return(fakeContentType)
	fakeContentType.On("With", []string{"content_type", "json"}).Return(fakeContentType)
	fakeContentType.On("With", []string{"content_type", "http"}).Return(fakeContentType)
	fakeContentType.On("With", []string{"content_type", "other"}).Return(fakeContentType)
	fakeContentType.On("Add", 1.0).Return()

	// QueueDepth case
	fakeQdepth := new(mockGauge)
	fakeQdepth.On("With", []string{"url", w.Webhook.Config.URL}).Return(fakeQdepth)
	fakeQdepth.On("Add", 1.0).Return().On("Add", -1.0).Return()

	// DropsDueToPanic case
	fakePanicDrop := new(mockCounter)
	fakePanicDrop.On("With", []string{"url", w.Webhook.Config.URL}).Return(fakePanicDrop)
	fakePanicDrop.On("Add", 1.0).Return()

	// Build a registry and register all fake metrics, these are synymous with the metrics in
	// metrics.go
	//
	// If a new metric within outboundsender is created it must be added here
	fakeRegistry := new(mockCaduceusMetricsRegistry)
	fakeRegistry.On("NewCounter", DeliveryRetryCounter).Return(fakeDC)
	fakeRegistry.On("NewCounter", DeliveryCounter).Return(fakeDC)
	fakeRegistry.On("NewCounter", OutgoingQueueDepth).Return(fakeDC)
	fakeRegistry.On("NewCounter", SlowConsumerCounter).Return(fakeSlow)
	fakeRegistry.On("NewCounter", SlowConsumerDroppedMsgCounter).Return(fakeDroppedSlow)
	fakeRegistry.On("NewCounter", DropsDueToPanic).Return(fakePanicDrop)
	fakeRegistry.On("NewGauge", OutgoingQueueDepth).Return(fakeQdepth)
	fakeRegistry.On("NewGauge", DeliveryRetryMaxGauge).Return(fakeQdepth)
	fakeRegistry.On("NewGauge", ConsumerRenewalTimeGauge).Return(fakeQdepth)
	fakeRegistry.On("NewGauge", ConsumerDeliverUntilGauge).Return(fakeQdepth)
	fakeRegistry.On("NewGauge", ConsumerDropUntilGauge).Return(fakeQdepth)
	fakeRegistry.On("NewGauge", ConsumerDeliveryWorkersGauge).Return(fakeQdepth)
	fakeRegistry.On("NewGauge", ConsumerMaxDeliveryWorkersGauge).Return(fakeQdepth)

	return &OutboundSenderFactory{
		Listener:        w,
		Sender:          (&http.Client{Transport: trans}).Do,
		CutOffPeriod:    cutOffPeriod,
		NumWorkers:      10,
		QueueSize:       10,
		DeliveryRetries: 1,
		MetricsRegistry: fakeRegistry,
		Logger:          log.NewNopLogger(),
	}
}

func simpleRequest() *wrp.Message {
	return &wrp.Message{
		Source:          "mac:112233445566/lmlite",
		TransactionUUID: "1234",
		ContentType:     wrp.MimeTypeMsgpack,
		Destination:     "event:bob/magic/dog",
		Payload:         []byte("Hello, world."),
	}
}

func simpleRequestWithPartnerIDs() *wrp.Message {
	return &wrp.Message{
		Source:          "mac:112233445566/lmlite",
		TransactionUUID: "1234",
		ContentType:     wrp.MimeTypeMsgpack,
		Destination:     "event:bob/magic/dog",
		Payload:         []byte("Hello, world."),
		PartnerIDs:      []string{"comcast"},
	}
}

// Simple test that covers the normal successful case with no extra matchers
func TestSimpleWrp(t *testing.T) {
	fmt.Printf("\n\nTestingSimpleWRP:\n\n")

	assert := assert.New(t)

	trans := &transport{}

	fmt.Printf("SimpleSetup:\n")
	obs, err := simpleSetup(trans, time.Second, nil)
	assert.NotNil(obs)
	assert.Nil(err)

	// queue case 1
	req := simpleRequestWithPartnerIDs()
	req.Destination = "event:iot"
	fmt.Printf("Queue case 1:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	req = simpleRequestWithPartnerIDs()
	req.Destination = "event:test"
	fmt.Printf("\nQueue case 2:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	// queue case 3
	req = simpleRequestWithPartnerIDs()
	req.Destination = "event:no-match"
	fmt.Printf("\nQueue case 3:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	// queue case 4
	req = simpleRequestWithPartnerIDs()
	req.ContentType = wrp.MimeTypeJson
	fmt.Printf("\nQueue case 3:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	req = simpleRequestWithPartnerIDs()
	req.ContentType = "application/http"
	fmt.Printf("\nQueue case 4:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	req = simpleRequestWithPartnerIDs()
	req.ContentType = "unknown"
	fmt.Printf("\nQueue case 4:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

func TestSimpleWrpPartnerIDsFailure(t *testing.T) {
	fmt.Printf("\n\nTestingSimpleWRP:\n\n")

	assert := assert.New(t)

	trans := &transport{}

	fmt.Printf("SimpleSetup:\n")
	obs, err := simpleSetup(trans, time.Second, nil)
	assert.NotNil(obs)
	assert.Nil(err)

	// queue case 1
	req := simpleRequest()
	req.Destination = "event:iot"
	fmt.Printf("Queue case 1:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	req = simpleRequest()
	req.Destination = "event:test"
	fmt.Printf("\nQueue case 2:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	// queue case 3
	req = simpleRequest()
	req.Destination = "event:no-match"
	fmt.Printf("\nQueue case 3:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	// queue case 4
	req = simpleRequest()
	req.ContentType = wrp.MimeTypeJson
	fmt.Printf("\nQueue case 3:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	req = simpleRequest()
	req.ContentType = "application/http"
	fmt.Printf("\nQueue case 4:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	req = simpleRequest()
	req.ContentType = "unknown"
	fmt.Printf("\nQueue case 4:\n %v\n", spew.Sprint(req))
	obs.Queue(req)

	obs.Shutdown(true)

	assert.Equal(int32(0), trans.i)
}

// Simple test that covers the normal retry case
func TestSimpleRetry(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (*http.Response, error) {
		return nil, &net.DNSError{IsTemporary: true}
	}

	obs, err := simpleSetup(trans, time.Second, nil)
	assert.NotNil(obs)
	assert.Nil(err)

	req := simpleRequestWithPartnerIDs()
	req.Source = "mac:112233445566"
	req.TransactionUUID = "1234"
	req.Destination = "event:iot"
	obs.Queue(req)

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

func Test429Retry(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (*http.Response, error) {
		return &http.Response{StatusCode: 429}, nil
	}

	obs, err := simpleSetup(trans, time.Second, nil)

	assert.NotNil(obs)
	assert.Nil(err)

	req := simpleRequestWithPartnerIDs()
	req.Source = "mac:112233445566"
	req.TransactionUUID = "1234"
	req.Destination = "event:iot"
	obs.Queue(req)

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

func TestAltURL(t *testing.T) {
	assert := assert.New(t)

	urls := map[string]int{}

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now().Add(60 * time.Second),
			Events: []string{".*"},
		},
		PartnerIDs: []string{"comcast"},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson
	w.Webhook.Config.AlternativeURLs = []string{
		"http://localhost:9999/foo",
		"http://localhost:9999/bar",
		"http://localhost:9999/faa",
		"http://localhost:9999/bas",
	}

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (*http.Response, error) {
		urls[req.URL.Path] += 1
		return &http.Response{StatusCode: 429}, nil
	}

	obs, err := simpleSetup(trans, time.Second, nil)
	assert.Nil(err)
	err = obs.Update(w)
	assert.NotNil(obs)
	assert.Nil(err)

	req := simpleRequestWithPartnerIDs()
	req.Source = "mac:112233445566"
	req.TransactionUUID = "1234"
	req.Destination = "event:iot"
	obs.Queue(req)

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
	for k, v := range urls {
		assert.Equal(1, v, k)
	}
}

// Simple test that covers the normal successful case with extra matchers
func TestSimpleWrpWithMatchers(t *testing.T) {

	assert := assert.New(t)

	m := []string{"mac:112233445566", "mac:112233445565"}

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleRequestWithPartnerIDs()
	req.TransactionUUID = "1234"
	req.Source = "mac:112233445566"
	req.Destination = "event:iot"
	obs.Queue(req)

	r2 := simpleRequestWithPartnerIDs()
	r2.TransactionUUID = "1234"
	r2.Source = "mac:112233445565"
	r2.Destination = "event:test"
	obs.Queue(r2)

	r3 := simpleRequest()
	r3.TransactionUUID = "1234"
	r3.Source = "mac:112233445560"
	r3.Destination = "event:iot"
	obs.Queue(r3)

	r4 := simpleRequest()
	r4.TransactionUUID = "1234"
	r4.Source = "mac:112233445560"
	r4.Destination = "event:test"
	obs.Queue(r4)

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

// Simple test that covers the normal successful case with extra wildcard matcher
func TestSimpleWrpWithWildcardMatchers(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}

	m := []string{"mac:112233445566", ".*"}

	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleRequestWithPartnerIDs()
	req.TransactionUUID = "1234"
	req.Source = "mac:112233445566"
	req.Destination = "event:iot"
	obs.Queue(req)

	r2 := simpleRequestWithPartnerIDs()
	r2.TransactionUUID = "1234"
	r2.Source = "mac:112233445565"
	r2.Destination = "event:test"
	obs.Queue(r2)

	r3 := simpleRequestWithPartnerIDs()
	r3.TransactionUUID = "1234"
	r3.Source = "mac:112233445560"
	r3.Destination = "event:iot"
	obs.Queue(r3)

	r4 := simpleRequestWithPartnerIDs()
	r4.TransactionUUID = "1234"
	r4.Source = "mac:112233445560"
	r4.Destination = "event:test"
	obs.Queue(r4)

	/* This will panic. */
	r5 := simpleRequestWithPartnerIDs()
	r5.TransactionUUID = "1234"
	r5.Source = "mac:112233445560"
	r5.Destination = "event:test\xedoops"
	obs.Queue(r5)

	obs.Shutdown(true)

	assert.Equal(int32(4), trans.i)
}

/*
// Simple test that covers the normal successful case with extra matchers
func TestSimpleWrpWithMetadata(t *testing.T) {

	assert := assert.New(t)

	m := make(map[string][]string)
	m["device_id"] = []string{"mac:112233445566", "mac:112233445565"}
	m["metadata"] = []string{"cheese", "crackers"}

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleRequest()

	wrpMeta := make(map[string]string)
	wrpMeta["metadata"] = "crackers"

	obs.Queue(req, wrpMeta, "iot", "mac:112233445565", "1234")
	obs.Queue(req, wrpMeta, "test", "mac:112233445566", "1234")
	obs.Queue(req, wrpMeta, "iot", "mac:112233445560", "1234")
	obs.Queue(req, wrpMeta, "test", "mac:112233445560", "1234")

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}
*/ /*
// Simple test that covers the normal successful case with extra matchers
func TestInvalidWrpMetadata(t *testing.T) {

	assert := assert.New(t)

	m := make(map[string][]string)
	m["device_id"] = []string{"mac:112233445566", "mac:112233445565"}
	m["metadata"] = []string{"cheese", "crackers"}

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleRequest()

	wrpMeta := make(map[string]string)
	wrpMeta["metadata"] = "notpresent"

	obs.Queue(req, wrpMeta, "iot", "mac:112233445565", "1234")
	obs.Queue(req, wrpMeta, "test", "mac:112233445566", "1234")
	obs.Queue(req, wrpMeta, "iot", "mac:112233445560", "1234")
	obs.Queue(req, wrpMeta, "test", "mac:112233445560", "1234")

	obs.Shutdown(true)

	assert.Equal(int32(0), trans.i)
}
*/

// Simple test that checks for invalid match regex
func TestInvalidMatchRegex(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}

	m := []string{"[[:112233445566"}

	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for invalid cutoff period
func TestInvalidCutOffPeriod(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}

	obs, err := simpleSetup(trans, 0*time.Second, nil)
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for invalid event regex
func TestInvalidEventRegex(t *testing.T) {

	assert := assert.New(t)

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now().Add(60 * time.Second),
			Events: []string{"[[:123"},
		},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	obs, err := OutboundSenderFactory{
		Listener:   w,
		Sender:     (&http.Client{}).Do,
		NumWorkers: 10,
		QueueSize:  10,
		Logger:     log.NewNopLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid url regex
func TestInvalidUrl(t *testing.T) {

	assert := assert.New(t)

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now().Add(60 * time.Second),
			Events: []string{"iot"},
		},
	}
	w.Webhook.Config.URL = "invalid"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	obs, err := OutboundSenderFactory{
		Listener:   w,
		Sender:     (&http.Client{}).Do,
		NumWorkers: 10,
		QueueSize:  10,
		Logger:     log.NewNopLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

	w2 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now().Add(60 * time.Second),
			Events: []string{"iot"},
		},
	}
	w2.Webhook.Config.ContentType = wrp.MimeTypeJson

	obs, err = OutboundSenderFactory{
		Listener:   w2,
		Sender:     (&http.Client{}).Do,
		NumWorkers: 10,
		QueueSize:  10,
		Logger:     log.NewNopLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid Sender
func TestInvalidSender(t *testing.T) {
	assert := assert.New(t)

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Sender = nil
	obs, err := obsf.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for no logger
func TestInvalidLogger(t *testing.T) {
	assert := assert.New(t)

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now().Add(60 * time.Second),
			Events: []string{"iot"},
		},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w
	obsf.Sender = (&http.Client{}).Do
	obsf.Logger = nil
	obs, err := obsf.New()

	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for FailureURL behavior
func TestFailureURL(t *testing.T) {
	assert := assert.New(t)

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:      time.Now().Add(60 * time.Second),
			FailureURL: "invalid",
			Events:     []string{"iot"},
		},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w
	obsf.Sender = (&http.Client{}).Do
	obs, err := obsf.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for no events
func TestInvalidEvents(t *testing.T) {
	assert := assert.New(t)

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until: time.Now().Add(60 * time.Second),
		},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w
	obsf.Sender = (&http.Client{}).Do
	obs, err := obsf.New()

	assert.Nil(obs)
	assert.NotNil(err)

	w2 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now().Add(60 * time.Second),
			Events: []string{"iot(.*"},
		},
	}
	w2.Webhook.Config.URL = "http://localhost:9999/foo"
	w2.Webhook.Config.ContentType = wrp.MimeTypeJson

	obsf = simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w2
	obsf.Sender = (&http.Client{}).Do
	obs, err = obsf.New()

	assert.Nil(obs)
	assert.NotNil(err)
}

// TODO: improve test
// Simple test that ensures that Update() works
func TestUpdate(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	w1 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  now,
			Events: []string{"iot", "test"},
		},
	}
	w1.Webhook.Config.URL = "http://localhost:9999/foo"
	w1.Webhook.Config.ContentType = wrp.MimeTypeMsgpack

	later := time.Now().Add(30 * time.Second)
	w2 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  later,
			Events: []string{"more", "messages"},
		},
	}
	w2.Webhook.Config.URL = "http://localhost:9999/foo"
	w2.Webhook.Config.ContentType = wrp.MimeTypeMsgpack

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w1
	obsf.Sender = (&http.Client{}).Do
	obs, err := obsf.New()
	assert.Nil(err)

	if _, ok := obs.(*CaduceusOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a CaduceusOutboundSender.")
	}

	assert.Equal(now, obs.(*CaduceusOutboundSender).deliverUntil, "Delivery should match original value.")
	obs.Update(w2)
	assert.Equal(later, obs.(*CaduceusOutboundSender).deliverUntil, "Delivery should match new value.")

	obs.Shutdown(true)
}

// No FailureURL
func TestOverflowNoFailureURL(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	logger := getNewTestOutputLogger(&output)

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now(),
			Events: []string{"iot", "test"},
		},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w
	obsf.Logger = logger
	obsf.Sender = (&http.Client{}).Do
	obs, err := obsf.New()

	assert.Nil(err)

	if _, ok := obs.(*CaduceusOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a CaduceusOutboundSender.")
	}

	obs.(*CaduceusOutboundSender).queueOverflow()
	assert.NotNil(output.String())
}

// Valid FailureURL
func TestOverflowValidFailureURL(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	logger := getNewTestOutputLogger(&output)

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		assert.Equal("POST", req.Method)
		assert.Equal([]string{wrp.MimeTypeJson}, req.Header["Content-Type"])
		assert.Nil(req.Header["X-Webpa-Signature"])
		payload, _ := ioutil.ReadAll(req.Body)
		// There is a timestamp in the body, so it's not worth trying to do a string comparison
		assert.NotNil(payload)

		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:      time.Now(),
			FailureURL: "http://localhost:12345/bar",
			Events:     []string{"iot", "test"},
		},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w
	obsf.Logger = logger
	obs, err := obsf.New()
	assert.Nil(err)

	if _, ok := obs.(*CaduceusOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a CaduceusOutboundSender.")
	}

	obs.(*CaduceusOutboundSender).queueOverflow()
	assert.NotNil(output.String())
}

// Valid FailureURL with secret
func TestOverflowValidFailureURLWithSecret(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	logger := getNewTestOutputLogger(&output)

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		assert.Equal("POST", req.Method)
		assert.Equal([]string{wrp.MimeTypeJson}, req.Header["Content-Type"])
		// There is a timestamp in the body, so it's not worth trying to do a string comparison
		assert.NotNil(req.Header["X-Webpa-Signature"])
		payload, _ := ioutil.ReadAll(req.Body)
		assert.NotNil(payload)

		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:      time.Now(),
			FailureURL: "http://localhost:12345/bar",
			Events:     []string{"iot", "test"},
		},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson
	w.Webhook.Config.Secret = "123456"

	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w
	obsf.Logger = logger
	obs, err := obsf.New()
	assert.Nil(err)

	if _, ok := obs.(*CaduceusOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a CaduceusOutboundSender.")
	}

	obs.(*CaduceusOutboundSender).queueOverflow()
	assert.NotNil(output.String())
}

// Valid FailureURL, failed to send, error
func TestOverflowValidFailureURLError(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	logger := getNewTestOutputLogger(&output)

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		resp = nil
		err = fmt.Errorf("My Error.")
		return
	}

	w := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:      time.Now(),
			FailureURL: "http://localhost:12345/bar",
			Events:     []string{"iot", "test"},
		},
	}
	w.Webhook.Config.URL = "http://localhost:9999/foo"
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	obsf := simpleFactorySetup(trans, time.Second, nil)
	obsf.Listener = w
	obsf.Logger = logger
	obs, err := obsf.New()
	assert.Nil(err)

	if _, ok := obs.(*CaduceusOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a CaduceusOutboundSender.")
	}

	obs.(*CaduceusOutboundSender).queueOverflow()
	assert.NotNil(output.String())
}

// Valid Overflow case
func TestOverflow(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	logger := getNewTestOutputLogger(&output)

	block := int32(0)
	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		if req.URL.String() == "http://localhost:9999/foo" {
			assert.Equal([]string{"01234"}, req.Header["X-Webpa-Transaction-Id"])

			// Sleeping until we're told to return
			for 0 == atomic.LoadInt32(&block) {
				time.Sleep(time.Microsecond)
			}
		}

		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	w := ancla.Webhook{
		Until:      time.Now().Add(30 * time.Second),
		FailureURL: "http://localhost:12345/bar",
		Events:     []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = wrp.MimeTypeJson

	obsf := simpleFactorySetup(trans, 4*time.Second, nil)
	obsf.NumWorkers = 1
	obsf.QueueSize = 2
	obsf.Logger = logger
	obs, err := obsf.New()
	assert.Nil(err)

	req := simpleRequest()

	req.TransactionUUID = "01234"
	obs.Queue(req)
	req.TransactionUUID = "01235"
	obs.Queue(req)

	// give the worker a chance to pick up one from the queue
	time.Sleep(1 * time.Second)

	req.TransactionUUID = "01236"
	obs.Queue(req)
	req.TransactionUUID = "01237"
	obs.Queue(req)
	req.TransactionUUID = "01238"
	obs.Queue(req)
	atomic.AddInt32(&block, 1)
	obs.Shutdown(false)

	assert.NotNil(output.String())
}
