// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	//"github.com/stretchr/testify/mock"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

const testLocalhostURL = "http://localhost:9999/foo"

//const testLocalhostStreamURL = "http://localhost:8888/someStream"

// TODO Improve all of these tests

// Make a simple RoundTrip implementation that let's me short-circuit the network
type transport struct {
	i  int32
	fn func(*http.Request, int) (*http.Response, error)
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	i := atomic.AddInt32(&t.i, 1)
	return t.fn(req, int(i))
}

func getNewTestOutputLogger(out io.Writer) *zap.Logger {
	var b bytes.Buffer

	return zap.New(
		zapcore.NewCore(zapcore.NewJSONEncoder(
			zapcore.EncoderConfig{
				MessageKey: "message",
			}), zapcore.AddSync(&b), zapcore.ErrorLevel),
	)
}

func simpleSetup(trans *transport, cutOffPeriod time.Duration, matcher []string) (OutboundSender, error) {
	return simpleFactorySetup(trans, cutOffPeriod, matcher, false).New()
}

// simpleFactorySetup sets up a outboundSender with metrics.
//
// # Using Caduceus's test suite
//
// If you are testing a new metric it needs to be created in this process below.
//  1. Create a fake, mockMetric i.e fakeEventType := new(mockCounter)
//  2. If your metric type has yet to be included in mockCaduceusMetricRegistry within mocks.go
//     add your metric type to mockCaduceusMetricRegistry
//  3. Trigger the On method on that "mockMetric" with various different cases of that metric,
//     in both senderWrapper_test.go and outboundSender_test.go
//     i.e:
//     case 1: On("With", []string{eventLabel, iot}
//     case 2: On("With", []string{eventLabel, unknown}
//  4. Mimic the metric behavior using On:
//     fakeSlow.On("Add", 1.0).Return()
func simpleFactorySetup(trans *transport, cutOffPeriod time.Duration, matcher []string, stream bool) *OutboundSenderFactory {
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
				URL:         testLocalhostURL,
				ContentType: wrp.MimeTypeJson,
				Secret:      "123456",
				AlternativeURLs: []string{
					testLocalhostURL,
					"http://localhost:9999/bar",
					"http://localhost:9999/faa",
					"http://localhost:9999/bas",
				},
			},
		},
		PartnerIDs: []string{"comcast"},
	}
	w.Webhook.Matcher.DeviceID = matcher

	// test dc metric
	fakeDC := new(mockCounter)
	fakeDC.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "200", eventLabel: "test", reasonLabel: noErrReason}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "200", eventLabel: "testoops", reasonLabel: noErrReason}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "200", eventLabel: "iot", reasonLabel: noErrReason}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: messageDroppedCode, eventLabel: "iot", reasonLabel: dnsErrReason}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "200", eventLabel: "unknown"}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "failure", eventLabel: "iot"}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, eventLabel: "test"}).Return(fakeDC).
		On("MustCurryWith", prometheus.Labels{urlLabel: w.Webhook.Config.URL, eventLabel: "test"}).Return(fakeDC).
		On("MustCurryWith", prometheus.Labels{urlLabel: w.Webhook.Config.URL, eventLabel: "test\xedoops"}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, eventLabel: "iot"}).Return(fakeDC).
		On("MustCurryWith", prometheus.Labels{urlLabel: w.Webhook.Config.URL, eventLabel: "iot"}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, eventLabel: "unknown"}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "201", reasonLabel: noErrReason}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "202", reasonLabel: noErrReason}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "204", reasonLabel: noErrReason}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "429", eventLabel: "iot", reasonLabel: noErrReason}).Return(fakeDC).
		On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "failure"}).Return(fakeDC)
	fakeDC.On("Add", 1.0).Return()
	fakeDC.On("Add", 0.0).Return()

	// test slow metric
	fakeSlow := new(mockCounter)
	fakeSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL}).Return(fakeSlow)
	fakeSlow.On("Add", 1.0).Return()

	// test dropped metric
	fakeDroppedSlow := new(mockCounter)
	fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, reasonLabel: "queue_full"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, reasonLabel: dnsErrReason}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, reasonLabel: "cut_off"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, reasonLabel: "expired"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, reasonLabel: "expired_before_queueing"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, reasonLabel: "invalid_config"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, reasonLabel: "network_err"}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, reasonLabel: DropsDueToPanic}).Return(fakeDroppedSlow)
	fakeDroppedSlow.On("Add", mock.Anything).Return()

	// IncomingContentType cases
	fakeContentType := new(mockCounter)
	fakeContentType.On("With", prometheus.Labels{"content_type": "msgpack"}).Return(fakeContentType)
	fakeContentType.On("With", prometheus.Labels{"content_type": "json"}).Return(fakeContentType)
	fakeContentType.On("With", prometheus.Labels{"content_type": "http"}).Return(fakeContentType)
	fakeContentType.On("With", prometheus.Labels{"content_type": "other"}).Return(fakeContentType)
	fakeContentType.On("Add", 1.0).Return()

	// QueueDepth case
	fakeQdepth := new(mockGauge)
	fakeQdepth.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL}).Return(fakeQdepth)
	fakeQdepth.On("Add", 1.0).Return().On("Add", -1.0).Return()
	fakeQdepth.On("Set", mock.Anything).Return()

	// Fake Latency
	fakeLatency := new(mockHistogram)
	fakeLatency.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "200", reasonLabel: noErrReason}).Return(fakeLatency)
	fakeLatency.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: "429", reasonLabel: noErrReason}).Return(fakeLatency)
	for _, url := range w.Webhook.Config.AlternativeURLs {
		fakeDC.On("With", prometheus.Labels{urlLabel: url, codeLabel: messageDroppedCode, eventLabel: "iot", reasonLabel: dnsErrReason}).Return(fakeDC)
		fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: url, reasonLabel: DropsDueToPanic}).Return(fakeDroppedSlow)
		fakeDC.On("With", prometheus.Labels{urlLabel: url, codeLabel: "200", eventLabel: "test\xedoops", reasonLabel: noErrReason}).Return(fakeDC)
		fakeDC.On("MustCurryWith", prometheus.Labels{urlLabel: w.Webhook.Config.URL, eventLabel: "test\xedoops"}).Return(fakeDC)
		fakeDC.On("With", prometheus.Labels{urlLabel: url, codeLabel: "429", eventLabel: "iot", reasonLabel: noErrReason}).Return(fakeDC)
		fakeLatency.On("With", prometheus.Labels{urlLabel: url, codeLabel: "429", reasonLabel: noErrReason}).Return(fakeLatency)
		fakeLatency.On("With", prometheus.Labels{urlLabel: url, codeLabel: "200", reasonLabel: noErrReason}).Return(fakeLatency)
		fakeDC.On("With", prometheus.Labels{urlLabel: url, codeLabel: "200", eventLabel: "test", reasonLabel: noErrReason}).Return(fakeDC)
		fakeDC.On("With", prometheus.Labels{urlLabel: url, codeLabel: "200", eventLabel: "iot", reasonLabel: noErrReason}).Return(fakeDC)
		fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: url, reasonLabel: dnsErrReason}).Return(fakeDroppedSlow)
		fakeDroppedSlow.On("With", prometheus.Labels{urlLabel: url, reasonLabel: "queue_full"}).Return(fakeDroppedSlow)
		fakeLatency.On("With", prometheus.Labels{urlLabel: url, codeLabel: unknown, reasonLabel: dnsErrReason}).Return(fakeLatency)
	}
	fakeLatency.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL, codeLabel: unknown, reasonLabel: dnsErrReason}).Return(fakeLatency)
	fakeLatency.On("With", prometheus.Labels{urlLabel: w.Webhook.Config.URL}).Return(fakeLatency)
	fakeLatency.On("Observe", mock.Anything).Return()

	return &OutboundSenderFactory{
		Metrics: OutboundSenderMetrics{
			queryLatency:          fakeLatency,
			deliveryCounter:       fakeDC,
			deliveryRetryCounter:  fakeDC,
			droppedMessage:        fakeDroppedSlow,
			cutOffCounter:         fakeSlow,
			queueDepthGauge:       fakeQdepth,
			renewalTimeGauge:      fakeQdepth,
			deliverUntilGauge:     fakeQdepth,
			dropUntilGauge:        fakeQdepth,
			maxWorkersGauge:       fakeQdepth,
			currentWorkersGauge:   fakeQdepth,
			deliveryRetryMaxGauge: fakeQdepth,
		},
		Listener:        w,
		Sender:          doerFunc((&http.Client{Transport: trans}).Do),
		CutOffPeriod:    cutOffPeriod,
		NumWorkers:      10,
		QueueSize:       10,
		DeliveryRetries: 1,
		Logger:          zap.NewNop(),
		IsStream:        stream,
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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson
	w.Webhook.Config.AlternativeURLs = []string{
		testLocalhostURL,
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

	// test Invaild UTF8
	r5 := simpleRequestWithPartnerIDs()
	r5.TransactionUUID = "1234"
	r5.Source = "mac:112233445560"
	r5.Destination = "event:test\xed"
	obs.Queue(r5)

	obs.Shutdown(true)

	assert.Equal(int32(5), trans.i)
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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	obs, err := OutboundSenderFactory{
		Listener:   w,
		Sender:     doerFunc((&http.Client{}).Do),
		NumWorkers: 10,
		QueueSize:  10,
		Logger:     zap.NewNop(),
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
		Sender:     doerFunc((&http.Client{}).Do),
		NumWorkers: 10,
		QueueSize:  10,
		Logger:     zap.NewNop(),
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
		Sender:     doerFunc((&http.Client{}).Do),
		NumWorkers: 10,
		QueueSize:  10,
		Logger:     zap.NewNop(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid Sender
func TestInvalidSender(t *testing.T) {
	assert := assert.New(t)

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil, false)
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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w
	obsf.Sender = doerFunc((&http.Client{}).Do)
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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w
	obsf.Sender = doerFunc((&http.Client{}).Do)
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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w
	obsf.Sender = doerFunc((&http.Client{}).Do)
	obs, err := obsf.New()

	assert.Nil(obs)
	assert.NotNil(err)

	w2 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  time.Now().Add(60 * time.Second),
			Events: []string{"iot(.*"},
		},
	}
	w2.Webhook.Config.URL = testLocalhostURL
	w2.Webhook.Config.ContentType = wrp.MimeTypeJson

	obsf = simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w2
	obsf.Sender = doerFunc((&http.Client{}).Do)
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
	w1.Webhook.Config.URL = testLocalhostURL
	w1.Webhook.Config.ContentType = wrp.MimeTypeMsgpack

	later := time.Now().Add(30 * time.Second)
	w2 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Until:  later,
			Events: []string{"more", "messages"},
		},
	}
	w2.Webhook.Config.URL = testLocalhostURL
	w2.Webhook.Config.ContentType = wrp.MimeTypeMsgpack

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w1
	obsf.Sender = doerFunc((&http.Client{}).Do)
	obs, err := obsf.New()
	assert.Nil(err)

	if _, ok := obs.(*webhookOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a WebhookOutboundSender.")
	}

	assert.Equal(now, obs.(*webhookOutboundSender).obs.deliverUntil, "Delivery should match original value.")
	obs.Update(w2)
	assert.Equal(later, obs.(*webhookOutboundSender).obs.deliverUntil, "Delivery should match new value.")

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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	trans := &transport{}
	obsf := simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w
	obsf.Logger = logger
	obsf.Sender = doerFunc((&http.Client{}).Do)
	obs, err := obsf.New()

	assert.Nil(err)

	if _, ok := obs.(*webhookOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a WebhookOutboundSender.")
	}

	//obs.QueueOverflow()
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
		payload, _ := io.ReadAll(req.Body)
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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	obsf := simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w
	obsf.Logger = logger
	obs, err := obsf.New()
	assert.Nil(err)

	if _, ok := obs.(*webhookOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a WebhookOutboundSender.")
	}

	obs.(*webhookOutboundSender).obs.dispatcher.QueueOverflow()

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
		payload, _ := io.ReadAll(req.Body)
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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson
	w.Webhook.Config.Secret = "123456"

	obsf := simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w
	obsf.Logger = logger
	obs, err := obsf.New()
	assert.Nil(err)

	if _, ok := obs.(*webhookOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a WebhookOutboundSender.")
	}

	obs.(*webhookOutboundSender).obs.dispatcher.QueueOverflow()
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
	w.Webhook.Config.URL = testLocalhostURL
	w.Webhook.Config.ContentType = wrp.MimeTypeJson

	obsf := simpleFactorySetup(trans, time.Second, nil, false)
	obsf.Listener = w
	obsf.Logger = logger
	obs, err := obsf.New()
	assert.Nil(err)

	if _, ok := obs.(*webhookOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a WebhookOutboundSender.")
	}

	obs.(*webhookOutboundSender).obs.dispatcher.QueueOverflow()

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
		if req.URL.String() == testLocalhostURL {
			assert.Equal([]string{"01234"}, req.Header["X-Webpa-Transaction-Id"])

			// Sleeping until we're told to return
			for atomic.LoadInt32(&block) == 0 {
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
	w.Config.URL = testLocalhostURL
	w.Config.ContentType = wrp.MimeTypeJson

	obsf := simpleFactorySetup(trans, 4*time.Second, nil, false)
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
