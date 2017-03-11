package main

import (
	//"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"
)

// Make a simple RoundTrip implementation that let's me short-circuit the network
type transport struct {
	i  int
	fn func(*http.Request, int) (*http.Response, error)
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	r, err := t.fn(req, t.i)
	t.i++
	return r, err
}

// Simple test that covers the normal successful case with no extra matchers
func TestSimple(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	obs, err := OutboundSenderFactory{
		Url:         "http://localhost:9999/foo",
		ContentType: "application/json",
		Client:      &http.Client{Transport: trans},
		Secret:      "123456",
		Until:       time.Now().Unix() + 60,
		Events:      []string{"iot", "test"},
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(err)

	req := CaduceusRequest{
		Payload:     []byte("Hello, world."),
		ContentType: "application/json",
		TargetURL:   "http://foo.com/api/v2/notification/device/mac:112233445566/event/iot",
	}
	obs.QueueJson(req, "iot", "mac:112233445566", "1234")
	obs.QueueJson(req, "test", "mac:112233445566", "1234")
	obs.QueueJson(req, "no-match", "mac:112233445566", "1234")

	obs.Shutdown(true)

	assert.Equal(2, trans.i)
}

// Simple test that covers the normal successful case with extra matchers
func TestSimpleWithMatchers(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	m := make(map[string][]string)
	m["device_id"] = []string{"mac:112233445566", "mac:112233445565"}

	obs, err := OutboundSenderFactory{
		Url:         "http://localhost:9999/foo",
		ContentType: "application/json",
		Client:      &http.Client{Transport: trans},
		Until:       time.Now().Unix() + 60,
		Events:      []string{"iot", "test"},
		Matchers:    m,
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(err)

	req := CaduceusRequest{
		Payload:     []byte("Hello, world."),
		ContentType: "application/json",
		TargetURL:   "http://foo.com/api/v2/notification/device/mac:112233445566/event/iot",
	}
	obs.QueueJson(req, "iot", "mac:112233445565", "1234")
	obs.QueueJson(req, "test", "mac:112233445566", "1234")
	obs.QueueJson(req, "iot", "mac:112233445560", "1234")
	obs.QueueJson(req, "test", "mac:112233445560", "1234")

	obs.Shutdown(true)

	assert.Equal(2, trans.i)
}

// Simple test that covers the normal successful case with extra wildcard matcher
func TestSimpleWithWildcardMatchers(t *testing.T) {

	assert := assert.New(t)

	trans := &transport{}
	trans.fn = func(req *http.Request, count int) (resp *http.Response, err error) {
		resp = &http.Response{Status: "200 OK",
			StatusCode: 200,
		}
		return
	}

	m := make(map[string][]string)
	m["device_id"] = []string{"mac:112233445566", ".*"}

	obs, err := OutboundSenderFactory{
		Url:         "http://localhost:9999/foo",
		ContentType: "application/json",
		Client:      &http.Client{Transport: trans},
		Until:       time.Now().Unix() + 60,
		Events:      []string{"iot", "test"},
		Matchers:    m,
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(err)

	req := CaduceusRequest{
		Payload:     []byte("Hello, world."),
		ContentType: "application/json",
		TargetURL:   "http://foo.com/api/v2/notification/device/mac:112233445566/event/iot",
	}
	obs.QueueJson(req, "iot", "mac:112233445565", "1234")
	obs.QueueJson(req, "test", "mac:112233445566", "1234")
	obs.QueueJson(req, "iot", "mac:112233445560", "1234")
	obs.QueueJson(req, "test", "mac:112233445560", "1234")

	obs.Shutdown(true)

	assert.Equal(4, trans.i)
}

// Simple test that checks for invalid match regex
func TestInvalidMatchRegex(t *testing.T) {

	assert := assert.New(t)

	m := make(map[string][]string)
	m["device_id"] = []string{"[[:112233445566"}

	obs, err := OutboundSenderFactory{
		Url:         "http://localhost:9999/foo",
		ContentType: "application/json",
		Client:      &http.Client{},
		Until:       time.Now().Unix() + 60,
		Events:      []string{"iot", "test"},
		Matchers:    m,
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid event regex
func TestInvalidEventRegex(t *testing.T) {

	assert := assert.New(t)

	obs, err := OutboundSenderFactory{
		Url:         "http://localhost:9999/foo",
		ContentType: "application/json",
		Client:      &http.Client{},
		Until:       time.Now().Unix() + 60,
		Events:      []string{"[[:123"},
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid url regex
func TestInvalidUrl(t *testing.T) {

	assert := assert.New(t)

	obs, err := OutboundSenderFactory{
		Url:         "invalid",
		ContentType: "application/json",
		Client:      &http.Client{},
		Until:       time.Now().Unix() + 60,
		Events:      []string{"iot"},
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

	obs, err = OutboundSenderFactory{
		ContentType: "application/json",
		Client:      &http.Client{},
		Until:       time.Now().Unix() + 60,
		Events:      []string{"iot"},
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that checks for invalid Client
func TestInvalidClient(t *testing.T) {

	assert := assert.New(t)

	obs, err := OutboundSenderFactory{
		Url:         "http://localhost:9999/foo",
		ContentType: "application/json",
		Until:       time.Now().Unix() + 60,
		Events:      []string{"iot"},
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(obs)
	assert.NotNil(err)

}

// Simple test that ensures that Extend() only does that
func TestExtend(t *testing.T) {

	assert := assert.New(t)

	now := time.Now().Unix()
	obs, err := OutboundSenderFactory{
		Url:         "http://localhost:9999/foo",
		ContentType: "application/json",
		Client:      &http.Client{},
		Until:       now,
		Events:      []string{"iot", "test"},
		NumWorkers:  10,
		QueueSize:   10,
	}.New()
	assert.Nil(err)

	assert.Equal(now, obs.deliverUntil, "Delivery should match previous value.")
	obs.Extend(0)
	assert.Equal(now, obs.deliverUntil, "Delivery should match previous value.")
	obs.Extend(now + 10)
	assert.Equal(now+10, obs.deliverUntil, "Delivery should match new value.")

	obs.Shutdown(true)
}
