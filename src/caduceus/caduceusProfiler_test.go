package main

import (
	//"fmt"
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
	arguments := m.Called()
	if arguments.Get(0) == nil {
		return nil
	}

	return arguments.Get(0).([]interface{})
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
		testProfiler, err := testFactory.New("dogbert")
		assert.NotNil(testProfiler)
		assert.Nil(err)
	})

	t.Run("TestCaduceusProfilerFactoryNewInvalidParameters", func(t *testing.T) {
		require.NotNil(t, testFactory)
		testFactory.Frequency = 0
		testProfiler, err := testFactory.New("dogbert")
		assert.Nil(testProfiler)
		assert.NotNil(err)
	})
}

func TestCaduceusProfiler(t *testing.T) {
	assert := assert.New(t)
	testMsg := CaduceusTelemetry{PayloadSize: 12}
	testData := make([]interface{}, 0)
	testData = append(testData, testMsg)

	// channel that we'll send random stuff to to trigger things in the aggregate method
	testChan := make(chan time.Time, 1)
	var testFunc Tick
	testFunc = func(time.Duration) <-chan time.Time {
		return testChan
	}

	testWG := new(sync.WaitGroup)

	// used to mock out a ring that the server profiler uses
	fakeRing := new(mockRing)
	fakeRing.On("Add", mock.AnythingOfType("[]interface {}")).Run(
		func(args mock.Arguments) {
			testWG.Done()
		}).Once()
	fakeRing.On("Snapshot").Return(testData).Once()

	// what we'll use for most of the tests
	testProfiler := caduceusProfiler{
		name:         "catbert",
		frequency:    1,
		tick:         testFunc,
		profilerRing: fakeRing,
		inChan:       make(chan interface{}, 10),
		quit:         make(chan struct{}),
		rwMutex:      new(sync.RWMutex),
	}

	// start this up for later
	go testProfiler.aggregate(testProfiler.quit)

	t.Run("TestCaduceusProfilerSend", func(t *testing.T) {
		require.NotNil(t, testProfiler)
		err := testProfiler.Send(testMsg)
		assert.Nil(err)
	})

	t.Run("TestCaduceusProfilerSendFullQueue", func(t *testing.T) {
		fullQueueProfiler := caduceusProfiler{
			name:         "catbert",
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

		assert.Equal(1, len(testResults))
		assert.Equal(CaduceusTelemetry{PayloadSize: 12}, testResults[0].(CaduceusTelemetry))

		fakeRing.AssertExpectations(t)
	})

	testProfiler.Close()
}

/*
func TestCaduceusProfilerProcess(t *testing.T) {
	set := []CaduceusTelemetry{
		{
			PayloadSize:  100,
			TimeReceived: time.Unix(1000, 0),
			TimeSent:     time.Unix(1000, 10),
		},
		{
			PayloadSize:  200,
			TimeReceived: time.Unix(1010, 0),
			TimeSent:     time.Unix(1010, 12),
		},
	}

	cp := caduceusProfiler{
		name:         "catbert",
		frequency:    1,
		profilerRing: NewCaduceusRing(1),
		inChan:       make(chan interface{}, 1),
		quit:         make(chan struct{}),
		rwMutex:      new(sync.RWMutex),
	}

	inputSet := make([]interface{}, len(set))
	for i, v := range set {
		inputSet[i] = v
	}
	out := cp.process(inputSet)

	// TODO this is not a test, just a hack - fix me!
	fmt.Printf("out: %+v\n", out[0])
}
*/
