package main

import (
	"bytes"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/stretchr/testify/assert"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func swGetLogger() logging.Logger {
	loggerFactory := logging.DefaultLoggerFactory{}
	logger, _ := loggerFactory.NewLogger("test")

	return logger
}

func TestInvalidLinger(t *testing.T) {
	sw, err := SenderWrapperFactory{
		NumWorkersPerSender: 10,
		QueueSizePerSender:  10,
		CutOffPeriod:        30 * time.Second,
		Logger:              swGetLogger(),
		Linger:              0 * time.Second,
		ProfilerFactory: ServerProfilerFactory{
			Frequency: 10,
			Duration:  6,
			QueueSize: 100,
		},
	}.New()

	assert := assert.New(t)
	assert.Nil(sw)
	assert.NotNil(err)
}

func TestSwSimple(t *testing.T) {
	assert := assert.New(t)

	wrpMessage := wrp.SimpleRequestResponse{
		Source:          "mac:112233445566",
		Destination:     "wrp",
		TransactionUUID: "12345",
	}

	var buffer bytes.Buffer
	encoder := wrp.NewEncoder(&buffer, wrp.Msgpack)
	err := encoder.Encode(&wrpMessage)
	assert.Nil(err)

	iot := CaduceusRequest{
		Payload:     []byte("Hello, world."),
		ContentType: "application/json",
		TargetURL:   "http://foo.com/api/v2/notification/device/mac:112233445566/event/iot",
	}
	test := CaduceusRequest{
		Payload:     []byte("Hello, world."),
		ContentType: "application/json",
		TargetURL:   "http://foo.com/api/v2/notification/device/mac:112233445566/event/test",
	}
	wrp := CaduceusRequest{
		Payload:     buffer.Bytes(),
		ContentType: "application/wrp",
		TargetURL:   "http://foo.com/api/v2/notification/device/mac:112233445566/event/wrp",
	}

	trans := &swTransport{}

	sw, err := SenderWrapperFactory{
		NumWorkersPerSender: 10,
		QueueSizePerSender:  10,
		CutOffPeriod:        30 * time.Second,
		Client:              &http.Client{Transport: trans},
		Logger:              swGetLogger(),
		Linger:              1 * time.Second,
		ProfilerFactory: ServerProfilerFactory{
			Frequency: 10,
			Duration:  6,
			QueueSize: 100,
		},
	}.New()

	assert.Nil(err)
	assert.NotNil(sw)

	// No listeners

	sw.Queue(iot)
	sw.Queue(iot)
	sw.Queue(iot)

	assert.Equal(int32(0), trans.i)

	w1 := webhook.W{
		Until:      time.Now().Add(6 * time.Second),
		Events:     []string{"iot"},
	}
	w1.Config.URL         = "http://localhost:9999/foo"
	w1.Config.ContentType = "application/json"
	
	w2 := webhook.W{
		Until:      time.Now().Add(4 * time.Second),
		Events:     []string{"iot", "test", "wrp"},
	}
	w2.Config.URL         = "http://localhost:9999/foo"
	w2.Config.ContentType = "application/json"

	// Add 2 listeners
	list := []webhook.W{w1, w2}

	sw.Update(list)

	// Send iot message
	sw.Queue(iot)
	time.Sleep(time.Second)
	assert.Equal(int32(2), atomic.LoadInt32(&trans.i))

	// Send test message
	sw.Queue(test)
	time.Sleep(time.Second)
	assert.Equal(int32(3), atomic.LoadInt32(&trans.i))

	// Send wrp message
	sw.Queue(wrp)
	time.Sleep(time.Second)
	assert.Equal(int32(4), atomic.LoadInt32(&trans.i))

	// Wait for one to expire & send it again
	time.Sleep(2 * time.Second)
	sw.Queue(test)
	time.Sleep(time.Second)
	assert.Equal(int32(4), atomic.LoadInt32(&trans.i))
	
	w3 := webhook.W{
		Until:      time.Now().Add(5 * time.Second),
		Events:     []string{"iot"},
	}
	w3.Config.URL         = "http://localhost:9999/foo"
	w3.Config.ContentType = "application/json"
	
	// We get a registration
	list2 := []webhook.W{w3}
	sw.Update(list2)
	time.Sleep(time.Second)

	// Send iot
	sw.Queue(iot)

	sw.Shutdown(true)
	assert.Equal(int32(5), atomic.LoadInt32(&trans.i))
}
