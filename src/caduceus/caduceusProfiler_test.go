package main

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

// Begin mock declarations

type mockRing struct {
	mock.Mock
}

func (m *mockRing) Add(inValue interface{}) {
	m.Called(inValue)
}

func (m *mockRing) Snapshot() (values []interface{}) {
	m.Called()
	return nil
}

// Begin test functions

func TestCaduceusProfilerFactory(t *testing.T) {
	assert := assert.New(t)

	testFactory := ServerProfilerFactory{
		Frequency: 1,
		Duration:  2,
		QueueSize: 10,
	}

	t.Run("TestCaduceusProfilerFactoryNew", func(t *testing.T) {
		require.NotNil(t, testFactory)
		testProfiler := testFactory.New()
		assert.NotNil(testProfiler)
	})
}

func TestCaduceusProfiler(t *testing.T) {
	assert := assert.New(t)
	testMsg := "test"
	testData := make([]interface{}, 0)
	testData = append(testData, testMsg)

	testChan := make(chan time.Time, 1)
	var testFunc Tick
	testFunc = func(time.Duration) <-chan time.Time {
		return testChan
	}

	testWG := new(sync.WaitGroup)

	fakeRing := new(mockRing)
	fakeRing.On("Add", mock.AnythingOfType("[]interface {}")).Run(
		func(args mock.Arguments) {
			testWG.Done()
		}).Once()
	fakeRing.On("Snapshot").Return(testData).Once()

	testProfiler := caduceusProfiler{
		frequency:    1,
		tick:         testFunc,
		profilerRing: fakeRing,
		inChan:       make(chan interface{}, 10),
		quit:         make(chan struct{}),
		rwMutex:      new(sync.RWMutex),
	}

	go testProfiler.aggregate(testProfiler.quit)

	t.Run("TestCaduceusProfilerSend", func(t *testing.T) {
		require.NotNil(t, testProfiler)
		err := testProfiler.Send(testMsg)
		assert.Nil(err)
	})

	t.Run("TestCaduceusProfilerSendFullQueue", func(t *testing.T) {
		fullQueueProfiler := caduceusProfiler{
			frequency:    1,
			profilerRing: NewCaduceusRing(1),
			inChan:       make(chan interface{}, 1),
			quit:         make(chan struct{}),
			rwMutex:      new(sync.RWMutex),
		}

		require.NotNil(t, fullQueueProfiler)
		// first send gets stored on the channel
		err := fullQueueProfiler.Send(testMsg)
		assert.Nil(err)

		// second send can't be accepted because the channel's full
		err = fullQueueProfiler.Send(testMsg)
		assert.NotNil(err)
	})

	// check to see if the data that we put on to the queue earlier is still there
	t.Run("TestCaduceusProfilerReport", func(t *testing.T) {
		require.NotNil(t, testProfiler)
		testWG.Add(1)
		testChan <- time.Now()
		testWG.Wait()
		testResults := testProfiler.Report()

		t.Log(testResults)

		found := false
		for _, value := range testResults {
			if assertedValue, ok := value.(string); ok {
				t.Log(assertedValue)
				if assertedValue == testMsg {
					found = true
					break
				}
			}
		}

		if !found {
			assert.Fail("The data that we put on to the profiler was not picked up.")
		}

		fakeRing.AssertExpectations(t)
	})
}
