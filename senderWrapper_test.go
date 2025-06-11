// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/webpa-common/v2/adapter"

	"github.com/xmidt-org/wrp-go/v3"
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
		On("With", prometheus.Labels{urlLabel: testLocalhostURL}).Return(fakeGauge).
		On("Set", mock.Anything).Return()

	fakeSlow := new(mockCounter)
	fakeSlow.On("With", prometheus.Labels{urlLabel: testLocalhostURL}).Return(fakeSlow).
		On("Add", 1.0).Return(fakeSlow)
	// Fake Latency
	fakeLatency := new(mockHistogram)
	fakeLatency.On("With", prometheus.Labels{urlLabel: testLocalhostURL, codeLabel: "200"}).Return(fakeLatency)
	fakeLatency.On("With", prometheus.Labels{urlLabel: testLocalhostURL}).Return(fakeLatency)
	fakeLatency.On("Observe", 1.0).Return()

	fakeIgnore := new(mockCounter)
	fakeIgnore.On("Add", 1.0).Return().On("Add", 0.0).Return().
		On("With", prometheus.Labels{urlLabel: testLocalhostURL}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, eventLabel: unknownEventType}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "cut_off"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "queue_full"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "expired"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "expired_before_queueing"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "network_err"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "invalid_config"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "cut_off"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "queue_full"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "expired"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "expired_before_queueing"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "network_err"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, reasonLabel: "invalid_config"}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, codeLabel: "200", eventLabel: unknownEventType}).Return(fakeIgnore).
		On("With", prometheus.Labels{urlLabel: testLocalhostURL, codeLabel: "200", eventLabel: unknownEventType}).Return(fakeIgnore).
		On("With", prometheus.Labels{eventLabel: "iot"}).Return(fakeIgnore).
		On("With", prometheus.Labels{eventLabel: "test/extra-stuff"}).Return(fakeIgnore).
		On("With", prometheus.Labels{eventLabel: "bob/magic/dog"}).Return(fakeIgnore).
		On("With", prometheus.Labels{eventLabel: unknownEventType}).Return(fakeIgnore).
		On("With", prometheus.Labels{"content_type": "msgpack"}).Return(fakeIgnore).
		On("With", prometheus.Labels{"content_type": "json"}).Return(fakeIgnore).
		On("With", prometheus.Labels{"content_type": "http"}).Return(fakeIgnore).
		On("With", prometheus.Labels{"content_type": "other"}).Return(fakeIgnore)

	return &SenderWrapperFactory{
		NumWorkersPerSender: 10,
		QueueSizePerSender:  10,
		CutOffPeriod:        30 * time.Second,
		Logger:              adapter.DefaultLogger().Logger,
		Linger:              0 * time.Second,
		Metrics: SenderWrapperMetrics{
			eventType: fakeIgnore,
		},
		outboundSenderMetrics: OutboundSenderMetrics{
			queryLatency:          fakeLatency,
			deliveryCounter:       fakeIgnore,
			deliveryRetryCounter:  fakeIgnore,
			renewalTimeGauge:      fakeGauge,
			deliverUntilGauge:     fakeGauge,
			dropUntilGauge:        fakeGauge,
			cutOffCounter:         fakeSlow,
			queueDepthGauge:       fakeGauge,
			maxWorkersGauge:       fakeGauge,
			currentWorkersGauge:   fakeGauge,
			deliveryRetryMaxGauge: fakeGauge,
		},
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

	wrpMessage := wrp.Message{
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
	swf.Sender = doerFunc((&http.Client{}).Do)

	swf.Linger = 1 * time.Second
	sw, err := swf.New()

	assert.Nil(err)
	assert.NotNil(sw)

	// No listeners
	sw.Queue(iot)
	sw.Queue(iot)
	sw.Queue(iot)

	assert.Equal(int32(0), trans.i)

	w1 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Config: ancla.DeliveryConfig{
				URL:         testLocalhostURL,
				ContentType: wrp.MimeTypeJson,
			},
			Duration: 6 * time.Second,
			Until:    time.Now().Add(6 * time.Second),
			Events:   []string{"iot"},
		},
	}
	w1.Webhook.Matcher.DeviceID = []string{"mac:112233445566"}

	w2 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Duration: 4 * time.Second,
			Until:    time.Now().Add(4 * time.Second),
			Events:   []string{"iot", "test/extra-stuff", "wrp"},
		},
	}
	w2.Webhook.Config.URL = testLocalhostURL
	w2.Webhook.Config.ContentType = wrp.MimeTypeJson
	w2.Webhook.Matcher.DeviceID = []string{"mac:112233445566"}

	// Add 2 listeners
	list := []ancla.InternalWebhook{w1, w2}

	sw.Update(list)

	// Send iot message

	sw.Queue(iot)

	// Send test message
	sw.Queue(test)

	// Send it again
	sw.Queue(test)

	w3 := ancla.InternalWebhook{
		Webhook: ancla.Webhook{},
	}
	w3.Webhook.Config.URL = testLocalhostURL
	w3.Webhook.Config.ContentType = wrp.MimeTypeJson

	// We get a registration
	list2 := []ancla.InternalWebhook{w3}
	sw.Update(list2)
	time.Sleep(time.Second)

	// Send iot
	sw.Queue(iot)

	sw.Shutdown(true)
	//assert.Equal(int32(4), atomic.LoadInt32(&trans.i))
}
