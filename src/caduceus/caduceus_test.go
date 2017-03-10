package main

import (
	"errors"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Begin mock declarations
type mockHandler struct {
	mock.Mock
}

func (m *mockHandler) HandleRequest(workerID int, inRequest CaduceusRequest) {
	return
}

type mockHealthTracker struct {
	mock.Mock
}

func (m *mockHealthTracker) SendEvent(health.HealthFunc) {
	return
}

func (m *mockHealthTracker) IncrementBucket(inSize int) {
	return
}

// Begin test functions
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestServeHTTP(t *testing.T) {
	assert := assert.New(t)

	logger := logging.DefaultLogger()
	fakeHandler := &mockHandler{}
	fakeHealth := &mockHealthTracker{}

	workerPool := WorkerPoolFactory{
		NumWorkers: 1,
		QueueSize:  1,
	}.New()

	serverWrapper := &ServerHandler{
		Logger:          logger,
		caduceusHandler: fakeHandler,
		caduceusHealth:  fakeHealth,
		doJob:           workerPool.Send,
	}

	t.Run("TestHappyPath", func(t *testing.T) {
		req := httptest.NewRequest("POST", "localhost:8080", strings.NewReader("Test payload."))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()

		serverWrapper.ServeHTTP(w, req)

		resp := w.Result()

		assert.Equal(202, resp.StatusCode)

		fakeHandler.AssertExpectations(t)
		fakeHealth.AssertExpectations(t)
	})

	t.Run("TestWrongHeader", func(t *testing.T) {
		req := httptest.NewRequest("POST", "localhost:8080", strings.NewReader("Test payload."))
		req.Header.Set("Bad-Header", "not/real")
		w := httptest.NewRecorder()

		serverWrapper.ServeHTTP(w, req)

		resp := w.Result()

		assert.Equal(400, resp.StatusCode)

		fakeHandler.AssertExpectations(t)
		fakeHealth.AssertExpectations(t)
	})

	t.Run("TestTooManyHeaders", func(t *testing.T) {
		req := httptest.NewRequest("POST", "localhost:8080", strings.NewReader("Test payload."))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Add("Content-Type", "too/many/headers")
		w := httptest.NewRecorder()

		serverWrapper.ServeHTTP(w, req)

		resp := w.Result()

		assert.Equal(400, resp.StatusCode)

		fakeHandler.AssertExpectations(t)
		fakeHealth.AssertExpectations(t)
	})

	t.Run("TestFullQueue", func(t *testing.T) {
		req := httptest.NewRequest("POST", "localhost:8080", strings.NewReader("Test payload."))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()

		requestTimeout := func(func(workerID int)) error {
			return errors.New("Intentional error.")
		}

		serverWrapper.doJob = requestTimeout
		serverWrapper.ServeHTTP(w, req)

		resp := w.Result()

		assert.Equal(408, resp.StatusCode)

		fakeHandler.AssertExpectations(t)
		fakeHealth.AssertExpectations(t)
	})
}
