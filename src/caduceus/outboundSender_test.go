package main

import (
	"bytes"
	"fmt"
	"github.com/Comcast/webpa-common/logging"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

var (
	testServerProfiler = ServerProfilerFactory{
		Frequency: 10,
		Duration:  6,
		QueueSize: 100,
	}.New()
)

// Make a simple RoundTrip implementation that let's me short-circuit the network
type transport struct {
	i  int32
	fn func(*http.Request, int) (*http.Response, error)
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	i := atomic.AddInt32(&t.i, 1)
	r, err := t.fn(req, int(i))
	return r, err
}

func getLogger() logging.Logger {
	loggerFactory := logging.DefaultLoggerFactory{}
	logger, _ := loggerFactory.NewLogger("test")

	return logger
}

func simpleSetup(trans *transport, cutOffPeriod time.Duration, matcher map[string][]string) (obs *OutboundSender, err error) {
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	obs, err = OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		ContentType:  "application/json",
		Client:       &http.Client{Transport: trans},
		Secret:       "123456",
		Until:        time.Now().Add(60 * time.Second),
		Events:       []string{"iot", "test"},
		Matchers:     matcher,
		CutOffPeriod: cutOffPeriod,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       getLogger(),
	}.New()
	return
}

func simpleRequest() CaduceusRequest {
	req := CaduceusRequest{
		Payload:     []byte("Hello, world."),
		ContentType: "application/json",
		TargetURL:   "http://foo.com/api/v2/notification/device/mac:112233445566/event/iot",
	}

	return req
}

// Simple test that covers the normal successful case with no extra matchers
func TestSimple(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, nil)
	assert.Nil(err)

	req := simpleRequest()

	obs.QueueJSON(req, "iot", "mac:112233445566", "1234")
	obs.QueueJSON(req, "test", "mac:112233445566", "1234")
	obs.QueueJSON(req, "no-match", "mac:112233445566", "1234")

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

// Simple test that covers the normal successful case with extra matchers
func TestSimpleWithMatchers(t *testing.T) {

	assert := assert.New(t)

	m := make(map[string][]string)
	m["device_id"] = []string{"mac:112233445566", "mac:112233445565"}

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleRequest()

	obs.QueueJSON(req, "iot", "mac:112233445565", "1234")
	obs.QueueJSON(req, "test", "mac:112233445566", "1234")
	obs.QueueJSON(req, "iot", "mac:112233445560", "1234")
	obs.QueueJSON(req, "test", "mac:112233445560", "1234")

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

// Simple test that covers the normal successful case with extra wildcard matcher
func TestSimpleWithWildcardMatchers(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}

	m := make(map[string][]string)
	m["device_id"] = []string{"mac:112233445566", ".*"}

	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleRequest()

	obs.QueueJSON(req, "iot", "mac:112233445565", "1234")
	obs.QueueJSON(req, "test", "mac:112233445566", "1234")
	obs.QueueJSON(req, "iot", "mac:112233445560", "1234")
	obs.QueueJSON(req, "test", "mac:112233445560", "1234")

	obs.Shutdown(true)

	assert.Equal(int32(4), trans.i)
}

// Simple test that checks for invalid match regex
func TestInvalidMatchRegex(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}

	m := make(map[string][]string)
	m["device_id"] = []string{"[[:112233445566"}

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

	obs, err := OutboundSenderFactory{
		URL:         "http://localhost:9999/foo",
		ContentType: "application/json",
		Client:      &http.Client{},
		Until:       time.Now().Add(60 * time.Second),
		Events:      []string{"[[:123"},
		NumWorkers:  10,
		QueueSize:   10,
		Profiler:    testServerProfiler,
		Logger:      getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid url regex
func TestInvalidUrl(t *testing.T) {

	assert := assert.New(t)

	obs, err := OutboundSenderFactory{
		URL:         "invalid",
		ContentType: "application/json",
		Client:      &http.Client{},
		Until:       time.Now().Add(60 * time.Second),
		Events:      []string{"iot"},
		NumWorkers:  10,
		QueueSize:   10,
		Profiler:    testServerProfiler,
		Logger:      getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

	obs, err = OutboundSenderFactory{
		ContentType: "application/json",
		Client:      &http.Client{},
		Until:       time.Now().Add(60 * time.Second),
		Events:      []string{"iot"},
		NumWorkers:  10,
		QueueSize:   10,
		Profiler:    testServerProfiler,
		Logger:      getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid Client
func TestInvalidClient(t *testing.T) {
	assert := assert.New(t)
	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		ContentType:  "application/json",
		Until:        time.Now().Add(60 * time.Second),
		Events:       []string{"iot"},
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for no logger
func TestInvalidLogger(t *testing.T) {
	assert := assert.New(t)
	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		Client:       &http.Client{},
		ContentType:  "application/json",
		Until:        time.Now().Add(60 * time.Second),
		Events:       []string{"iot"},
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for FailureURL behavior
func TestFailureURL(t *testing.T) {
	assert := assert.New(t)
	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		Client:       &http.Client{},
		ContentType:  "application/json",
		Until:        time.Now().Add(60 * time.Second),
		Events:       []string{"iot"},
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Logger:       getLogger(),
		FailureURL:   "invalid",
		Profiler:     testServerProfiler,
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for no events
func TestInvalidEvents(t *testing.T) {
	assert := assert.New(t)
	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		Client:       &http.Client{},
		ContentType:  "application/json",
		Until:        time.Now().Add(60 * time.Second),
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

	obs, err = OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		Client:       &http.Client{},
		ContentType:  "application/json",
		Until:        time.Now().Add(60 * time.Second),
		CutOffPeriod: time.Second,
		Events:       []string{"iot(.*"},
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that ensures that Extend() only does that
func TestExtend(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		ContentType:  "application/json",
		Client:       &http.Client{},
		Until:        now,
		Events:       []string{"iot", "test"},
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       getLogger(),
	}.New()
	assert.Nil(err)

	assert.Equal(now, obs.deliverUntil, "Delivery should match previous value.")
	obs.Extend(time.Time{})
	assert.Equal(now, obs.deliverUntil, "Delivery should match previous value.")
	extended := now.Add(10 * time.Second)
	obs.Extend(extended)
	assert.Equal(extended, obs.deliverUntil, "Delivery should match new value.")

	obs.Shutdown(true)
}

// No FailureURL
func TestOverflowNoFailureURL(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	loggerFactory := logging.DefaultLoggerFactory{&output}
	logger, _ := loggerFactory.NewLogger("test")

	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		ContentType:  "application/json",
		Client:       &http.Client{},
		Until:        time.Now(),
		Events:       []string{"iot", "test"},
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       logger,
	}.New()
	assert.Nil(err)

	obs.queueOverflow()
	assert.Equal("[ERROR] No cut-off notification URL specified.\n", output.String())
}

// Valid FailureURL
func TestOverflowValidFailureURL(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	loggerFactory := logging.DefaultLoggerFactory{&output}
	logger, _ := loggerFactory.NewLogger("test")

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		assert.Equal("POST", req.Method)
		assert.Equal([]string{"application/json"}, req.Header["Content-Type"])
		assert.Nil(req.Header["X-Webpa-Signature"])
		payload, _ := ioutil.ReadAll(req.Body)
		assert.Equal(`{"url":"http://localhost:9999/foo","text":"Unfortunately, your endpoint is not able to keep up with the traffic being sent to it.  Due to this circumstance, all notification traffic is being cut off and dropped for a period of time.  Please increase your capacity to handle notifications, or reduce the number of notifications you have requested.","events":["iot","test"],"cut-off-period":"1s","queue-size":10,"worker-count":10}`, string(payload))

		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		ContentType:  "application/json",
		Client:       &http.Client{Transport: trans},
		Until:        time.Now(),
		Events:       []string{"iot", "test"},
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       logger,
		FailureURL:   "http://localhost:12345/bar",
	}.New()
	assert.Nil(err)

	obs.queueOverflow()
	assert.Equal("[ERROR] Able to send cut-off notification (http://localhost:12345/bar) status: 200 OK\n", output.String())
}

// Valid FailureURL with secret
func TestOverflowValidFailureURLWithSecret(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	loggerFactory := logging.DefaultLoggerFactory{&output}
	logger, _ := loggerFactory.NewLogger("test")

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		assert.Equal("POST", req.Method)
		assert.Equal([]string{"application/json"}, req.Header["Content-Type"])
		assert.Equal([]string{"sha1=9a436f9f7c4722e5be86812456f65aa07bada4df"}, req.Header["X-Webpa-Signature"])
		payload, _ := ioutil.ReadAll(req.Body)
		assert.Equal(`{"url":"http://localhost:9999/foo","text":"Unfortunately, your endpoint is not able to keep up with the traffic being sent to it.  Due to this circumstance, all notification traffic is being cut off and dropped for a period of time.  Please increase your capacity to handle notifications, or reduce the number of notifications you have requested.","events":["iot","test"],"cut-off-period":"1s","queue-size":10,"worker-count":10}`, string(payload))

		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		ContentType:  "application/json",
		Client:       &http.Client{Transport: trans},
		Until:        time.Now(),
		Secret:       "123456",
		Events:       []string{"iot", "test"},
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       logger,
		FailureURL:   "http://localhost:12345/bar",
	}.New()
	assert.Nil(err)

	obs.queueOverflow()
	assert.Equal("[ERROR] Able to send cut-off notification (http://localhost:12345/bar) status: 200 OK\n", output.String())
}

// Valid FailureURL, failed to send, error
func TestOverflowValidFailureURLError(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	loggerFactory := logging.DefaultLoggerFactory{&output}
	logger, _ := loggerFactory.NewLogger("test")

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		resp = nil
		err = fmt.Errorf("My Error.")
		return
	}

	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		ContentType:  "application/json",
		Client:       &http.Client{Transport: trans},
		Until:        time.Now(),
		Events:       []string{"iot", "test"},
		CutOffPeriod: time.Second,
		NumWorkers:   10,
		QueueSize:    10,
		Profiler:     testServerProfiler,
		Logger:       logger,
		FailureURL:   "http://localhost:12345/bar",
	}.New()
	assert.Nil(err)

	obs.queueOverflow()
	assert.Equal("[ERROR] Unable to send cut-off notification (http://localhost:12345/bar) err: Post http://localhost:12345/bar: My Error.\n", output.String())
}

// Valid Overflow case
func TestOverflow(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	loggerFactory := logging.DefaultLoggerFactory{&output}
	logger, _ := loggerFactory.NewLogger("test")

	var block int32
	block = 0
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

	obs, err := OutboundSenderFactory{
		URL:          "http://localhost:9999/foo",
		ContentType:  "application/json",
		Client:       &http.Client{Transport: trans},
		Until:        time.Now().Add(30 * time.Second),
		Events:       []string{"iot", "test"},
		CutOffPeriod: 4 * time.Second,
		NumWorkers:   1,
		QueueSize:    2,
		Profiler:     testServerProfiler,
		Logger:       logger,
		FailureURL:   "http://localhost:12345/bar",
	}.New()
	assert.Nil(err)

	req := simpleRequest()

	obs.QueueJSON(req, "iot", "mac:112233445565", "01234")
	obs.QueueJSON(req, "iot", "mac:112233445565", "01235")

	// give the worker a chance to pick up one from the queue
	time.Sleep(1 * time.Second)

	obs.QueueJSON(req, "iot", "mac:112233445565", "01236")
	obs.QueueJSON(req, "iot", "mac:112233445565", "01237")
	obs.QueueJSON(req, "iot", "mac:112233445565", "01238")
	atomic.AddInt32(&block, 1)
	obs.Shutdown(false)

	assert.Equal("[ERROR] Able to send cut-off notification (http://localhost:12345/bar) status: 200 OK\n", output.String())

}
