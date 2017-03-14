package main

import (
	"errors"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// Begin mock declarations

// mockHandler only needs to mock the `HandleRequest` method
type mockHandler struct {
	mock.Mock
}

func (m *mockHandler) HandleRequest(workerID int, inRequest CaduceusRequest) {
	m.Called(workerID, inRequest)
}

// mockHealthTracker needs to mock things from both the `HealthTracker`
// interface as well as the `health.Monitor` interface
type mockHealthTracker struct {
	mock.Mock
}

func (m *mockHealthTracker) SendEvent(healthFunc health.HealthFunc) {
	m.Called(healthFunc)
}

func (m *mockHealthTracker) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	m.Called(response, request)
}

func (m *mockHealthTracker) IncrementBucket(inSize int) {
	m.Called(inSize)
}

// Begin test functions

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestWorkerPool(t *testing.T) {
	assert := assert.New(t)

	workerPool := WorkerPoolFactory{
		NumWorkers: 1,
		QueueSize:  1,
	}.New()

	t.Run("TestWorkerPoolSend", func(t *testing.T) {
		testWG := new(sync.WaitGroup)
		testWG.Add(1)

		require.NotNil(t, workerPool)
		err := workerPool.Send(func(workerID int) {
			testWG.Done()
		})

		testWG.Wait()
		assert.Nil(err)
	})

	workerPool = WorkerPoolFactory{
		NumWorkers: 0,
		QueueSize:  0,
	}.New()

	t.Run("TestWorkerPoolFullQueue", func(t *testing.T) {
		require.NotNil(t, workerPool)
		err := workerPool.Send(func(workerID int) {
			assert.Fail("This should not execute because our worker queue is full and we have no workers.")
		})

		assert.NotNil(err)
	})
}

func TestCaduceusHealth(t *testing.T) {
	assert := assert.New(t)

	testData := []struct {
		inSize       int
		expectedStat health.Stat
	}{
		{inSize: -1, expectedStat: PayloadsOverZero},
		{inSize: 0, expectedStat: PayloadsOverZero},
		{inSize: 99, expectedStat: PayloadsOverZero},
		{inSize: 999, expectedStat: PayloadsOverHundred},
		{inSize: 9999, expectedStat: PayloadsOverThousand},
		{inSize: 10001, expectedStat: PayloadsOverTenThousand},
		{inSize: math.MaxInt32, expectedStat: PayloadsOverTenThousand},
	}

	t.Run("TestIncrementBucket", func(t *testing.T) {
		for _, data := range testData {
			fakeMonitor := new(mockHealthTracker)
			fakeMonitor.On("SendEvent", mock.AnythingOfType("health.HealthFunc")).Run(
				func(args mock.Arguments) {
					healthFunc := args.Get(0).(health.HealthFunc)
					stats := make(health.Stats)

					healthFunc(stats)
					assert.Equal(1, stats[data.expectedStat])
				}).Once()

			caduceusHealth := &CaduceusHealth{fakeMonitor}

			caduceusHealth.IncrementBucket(data.inSize)
			fakeMonitor.AssertExpectations(t)
		}
	})
}

func TestServeHandler(t *testing.T) {
	assert := assert.New(t)

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"), mock.AnythingOfType("CaduceusRequest")).Return().Once()

	fakeHealth := new(mockHealthTracker)
	fakeHealth.On("IncrementBucket", mock.AnythingOfType("int")).Return().Once()

	requestSuccessful := func(func(workerID int)) error {
		fakeHandler.HandleRequest(0, CaduceusRequest{})
		return nil
	}

	serverWrapper := &ServerHandler{
		Logger:          logger,
		caduceusHandler: fakeHandler,
		caduceusHealth:  fakeHealth,
		doJob:           requestSuccessful,
	}

	req := httptest.NewRequest("POST", "localhost:8080", strings.NewReader("Test payload."))

	t.Run("TestServeHTTPHappyPath", func(t *testing.T) {
		req.Header.Set("Content-Type", "text/plain")

		w := httptest.NewRecorder()
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(202, resp.StatusCode)
		fakeHandler.AssertExpectations(t)
		fakeHealth.AssertExpectations(t)
	})

	t.Run("TestServeHTTPTooManyHeaders", func(t *testing.T) {
		req.Header.Add("Content-Type", "too/many/headers")

		w := httptest.NewRecorder()
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(400, resp.StatusCode)
		fakeHandler.AssertExpectations(t)
		fakeHealth.AssertExpectations(t)
	})

	t.Run("TestServeHTTPWrongHeader", func(t *testing.T) {
		req.Header.Del("Content-Type")

		w := httptest.NewRecorder()
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(400, resp.StatusCode)
		fakeHandler.AssertExpectations(t)
		fakeHealth.AssertExpectations(t)
	})

	t.Run("TestServeHTTPFullQueue", func(t *testing.T) {
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
