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
	"bytes"
	"fmt"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

var (
	testServerProfilerFactory = ServerProfilerFactory{
		Frequency: 10,
		Duration:  6,
		QueueSize: 100,
	}
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

func getLogger() log.Logger {
	return log.NewNopLogger()
}

func getNewTestOutputLogger(out io.Writer) log.Logger {
	return log.NewLogfmtLogger(out)
}

func simpleSetup(trans *transport, cutOffPeriod time.Duration, matcher []string) (obs OutboundSender, err error) {
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	w := webhook.W{
		Until:  time.Now().Add(60 * time.Second),
		Events: []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"
	w.Config.Secret = "123456"
	w.Matcher.DeviceId = matcher

	obs, err = OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{Transport: trans},
		CutOffPeriod:    cutOffPeriod,
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          getLogger(),
	}.New()
	return
}

func simpleJSONRequest() CaduceusRequest {
	req := CaduceusRequest{
		RawPayload:  []byte("Hello, world."),
		ContentType: "application/json",
		TargetURL:   "http://foo.com/api/v2/notification/device/mac:112233445566/event/iot",
	}

	return req
}

func simpleWrpRequest() CaduceusRequest {
	req := CaduceusRequest{
		RawPayload:   []byte("Hello, world."),
		PayloadAsWrp: &wrp.Message{},
		ContentType:  "application/wrp",
		TargetURL:    "http://foo.com/api/v2/notification/device/mac:112233445566/event/iot",
	}

	return req
}

// Simple test that covers the normal successful case with no extra matchers
func TestSimpleJSON(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, nil)
	assert.NotNil(obs)
	assert.Nil(err)

	req := simpleJSONRequest()

	obs.QueueJSON(req, "iot", "mac:112233445566", "1234")
	obs.QueueJSON(req, "test", "mac:112233445566", "1234")
	obs.QueueJSON(req, "no-match", "mac:112233445566", "1234")

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

// Simple test that covers the normal successful case with extra matchers
func TestSimpleJSONWithMatchers(t *testing.T) {

	assert := assert.New(t)

	m := []string{"mac:112233445566", "mac:112233445565"}

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleJSONRequest()

	obs.QueueJSON(req, "iot", "mac:112233445565", "1234")
	obs.QueueJSON(req, "test", "mac:112233445566", "1234")
	obs.QueueJSON(req, "iot", "mac:112233445560", "1234")
	obs.QueueJSON(req, "test", "mac:112233445560", "1234")

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

// Simple test that covers the normal successful case with extra wildcard matcher
func TestSimpleJSONWithWildcardMatchers(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}

	m := []string{"mac:112233445566", ".*"}

	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleJSONRequest()

	obs.QueueJSON(req, "iot", "mac:112233445565", "1234")
	obs.QueueJSON(req, "test", "mac:112233445566", "1234")
	obs.QueueJSON(req, "iot", "mac:112233445560", "1234")
	obs.QueueJSON(req, "test", "mac:112233445560", "1234")

	obs.Shutdown(true)

	assert.Equal(int32(4), trans.i)
}

// Simple test that covers the normal successful case with no extra matchers
func TestSimpleWrp(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, nil)
	assert.NotNil(obs)
	assert.Nil(err)

	req := simpleWrpRequest()

	req.PayloadAsWrp.Source = "mac:112233445566"
	req.PayloadAsWrp.TransactionUUID = "1234"

	req.PayloadAsWrp.Destination = "iot"
	obs.QueueWrp(req)

	req.PayloadAsWrp.Destination = "test"
	obs.QueueWrp(req)

	req.PayloadAsWrp.Destination = "no-match"
	obs.QueueWrp(req)

	obs.Shutdown(true)

	assert.Equal(int32(2), trans.i)
}

// Simple test that covers the normal successful case with extra matchers
func TestSimpleWrpWithMatchers(t *testing.T) {

	assert := assert.New(t)

	m := []string{"mac:112233445566", "mac:112233445565"}

	trans := &transport{}
	obs, err := simpleSetup(trans, time.Second, m)
	assert.Nil(err)

	req := simpleWrpRequest()

	req.PayloadAsWrp.TransactionUUID = "1234"

	req.PayloadAsWrp.Source = "mac:112233445566"
	req.PayloadAsWrp.Destination = "iot"
	obs.QueueWrp(req)

	req.PayloadAsWrp.Source = "mac:112233445565"
	req.PayloadAsWrp.Destination = "test"
	obs.QueueWrp(req)

	req.PayloadAsWrp.Source = "mac:112233445560"
	req.PayloadAsWrp.Destination = "iot"
	obs.QueueWrp(req)

	req.PayloadAsWrp.Source = "mac:112233445560"
	req.PayloadAsWrp.Destination = "test"
	obs.QueueWrp(req)

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

	req := simpleWrpRequest()

	req.PayloadAsWrp.TransactionUUID = "1234"

	req.PayloadAsWrp.Source = "mac:112233445566"
	req.PayloadAsWrp.Destination = "iot"
	obs.QueueWrp(req)

	req.PayloadAsWrp.Source = "mac:112233445565"
	req.PayloadAsWrp.Destination = "test"
	obs.QueueWrp(req)

	req.PayloadAsWrp.Source = "mac:112233445560"
	req.PayloadAsWrp.Destination = "iot"
	obs.QueueWrp(req)

	req.PayloadAsWrp.Source = "mac:112233445560"
	req.PayloadAsWrp.Destination = "test"
	obs.QueueWrp(req)

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

	req := simpleWrpRequest()

	wrpMeta := make(map[string]string)
	wrpMeta["metadata"] = "crackers"

	obs.QueueWrp(req, wrpMeta, "iot", "mac:112233445565", "1234")
	obs.QueueWrp(req, wrpMeta, "test", "mac:112233445566", "1234")
	obs.QueueWrp(req, wrpMeta, "iot", "mac:112233445560", "1234")
	obs.QueueWrp(req, wrpMeta, "test", "mac:112233445560", "1234")

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

	req := simpleWrpRequest()

	wrpMeta := make(map[string]string)
	wrpMeta["metadata"] = "notpresent"

	obs.QueueWrp(req, wrpMeta, "iot", "mac:112233445565", "1234")
	obs.QueueWrp(req, wrpMeta, "test", "mac:112233445566", "1234")
	obs.QueueWrp(req, wrpMeta, "iot", "mac:112233445560", "1234")
	obs.QueueWrp(req, wrpMeta, "test", "mac:112233445560", "1234")

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

	w := webhook.W{
		Until:  time.Now().Add(60 * time.Second),
		Events: []string{"[[:123"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{},
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid url regex
func TestInvalidUrl(t *testing.T) {

	assert := assert.New(t)

	w := webhook.W{
		Until:  time.Now().Add(60 * time.Second),
		Events: []string{"iot"},
	}
	w.Config.URL = "invalid"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{},
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

	w2 := webhook.W{
		Until:  time.Now().Add(60 * time.Second),
		Events: []string{"iot"},
	}
	w2.Config.ContentType = "application/json"

	obs, err = OutboundSenderFactory{
		Listener:        w2,
		Client:          &http.Client{},
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid Client
func TestInvalidClient(t *testing.T) {
	assert := assert.New(t)

	w := webhook.W{
		Until:  time.Now().Add(60 * time.Second),
		Events: []string{"iot"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for no logger
func TestInvalidLogger(t *testing.T) {
	assert := assert.New(t)

	w := webhook.W{
		Until:  time.Now().Add(60 * time.Second),
		Events: []string{"iot"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for FailureURL behavior
func TestFailureURL(t *testing.T) {
	assert := assert.New(t)

	w := webhook.W{
		Until:      time.Now().Add(60 * time.Second),
		FailureURL: "invalid",
		Events:     []string{"iot"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for no events
func TestInvalidEvents(t *testing.T) {
	assert := assert.New(t)

	w := webhook.W{
		Until: time.Now().Add(60 * time.Second),
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          getLogger(),
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

	w2 := webhook.W{
		Until:  time.Now().Add(60 * time.Second),
		Events: []string{"iot(.*"},
	}
	w2.Config.URL = "http://localhost:9999/foo"
	w2.Config.ContentType = "application/json"

	obs, err = OutboundSenderFactory{
		Listener:        w2,
		Client:          &http.Client{},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		Logger:          getLogger(),
		ProfilerFactory: testServerProfilerFactory,
	}.New()

	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that checks for no profiler
func TestInvalidProfilerFactory(t *testing.T) {
	assert := assert.New(t)

	w := webhook.W{
		Until:  time.Now(),
		Events: []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		Logger:          getLogger(),
		ProfilerFactory: ServerProfilerFactory{},
	}.New()

	assert.Nil(obs)
	assert.NotNil(err)
}

// Simple test that ensures that Extend() only does that
func TestExtend(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	w := webhook.W{
		Until:  now,
		Events: []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          getLogger(),
	}.New()
	assert.Nil(err)

	if _, ok := obs.(*CaduceusOutboundSender); !ok {
		assert.Fail("Interface returned by OutboundSenderFactory.New() must be implemented by a CaduceusOutboundSender.")
	}

	assert.Equal(now, obs.(*CaduceusOutboundSender).deliverUntil, "Delivery should match previous value.")
	obs.Extend(time.Time{})
	assert.Equal(now, obs.(*CaduceusOutboundSender).deliverUntil, "Delivery should match previous value.")
	extended := now.Add(10 * time.Second)
	obs.Extend(extended)
	assert.Equal(extended, obs.(*CaduceusOutboundSender).deliverUntil, "Delivery should match new value.")

	obs.Shutdown(true)
}

// No FailureURL
func TestOverflowNoFailureURL(t *testing.T) {
	assert := assert.New(t)

	var output bytes.Buffer
	logger := getNewTestOutputLogger(&output)

	w := webhook.W{
		Until:  time.Now(),
		Events: []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		Logger:          logger,
		ProfilerFactory: testServerProfilerFactory,
	}.New()
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
		assert.Equal([]string{"application/json"}, req.Header["Content-Type"])
		assert.Nil(req.Header["X-Webpa-Signature"])
		payload, _ := ioutil.ReadAll(req.Body)
		// There is a timestamp in the body, so it's not worth trying to do a string comparison
		assert.NotNil(payload)

		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	w := webhook.W{
		Until:      time.Now(),
		FailureURL: "http://localhost:12345/bar",
		Events:     []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{Transport: trans},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          logger,
	}.New()
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
		assert.Equal([]string{"application/json"}, req.Header["Content-Type"])
		// There is a timestamp in the body, so it's not worth trying to do a string comparison
		assert.NotNil(req.Header["X-Webpa-Signature"])
		payload, _ := ioutil.ReadAll(req.Body)
		assert.NotNil(payload)

		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	w := webhook.W{
		Until:      time.Now(),
		FailureURL: "http://localhost:12345/bar",
		Events:     []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"
	w.Config.Secret = "123456"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{Transport: trans},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          logger,
	}.New()
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

	w := webhook.W{
		Until:      time.Now(),
		FailureURL: "http://localhost:12345/bar",
		Events:     []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{Transport: trans},
		CutOffPeriod:    time.Second,
		NumWorkers:      10,
		QueueSize:       10,
		Logger:          logger,
		ProfilerFactory: testServerProfilerFactory,
	}.New()
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

	w := webhook.W{
		Until:      time.Now().Add(30 * time.Second),
		FailureURL: "http://localhost:12345/bar",
		Events:     []string{"iot", "test"},
	}
	w.Config.URL = "http://localhost:9999/foo"
	w.Config.ContentType = "application/json"

	obs, err := OutboundSenderFactory{
		Listener:        w,
		Client:          &http.Client{Transport: trans},
		CutOffPeriod:    4 * time.Second,
		NumWorkers:      1,
		QueueSize:       2,
		ProfilerFactory: testServerProfilerFactory,
		Logger:          logger,
	}.New()
	assert.Nil(err)

	req := simpleJSONRequest()

	obs.QueueJSON(req, "iot", "mac:112233445565", "01234")
	obs.QueueJSON(req, "iot", "mac:112233445565", "01235")

	// give the worker a chance to pick up one from the queue
	time.Sleep(1 * time.Second)

	obs.QueueJSON(req, "iot", "mac:112233445565", "01236")
	obs.QueueJSON(req, "iot", "mac:112233445565", "01237")
	obs.QueueJSON(req, "iot", "mac:112233445565", "01238")
	atomic.AddInt32(&block, 1)
	obs.Shutdown(false)

	assert.NotNil(output.String())
}
