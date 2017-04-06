package main

import (
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/stretchr/testify/mock"
	"net/http"
	"time"
)

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

// mockOutboundSender needs to mock things that the `OutboundSender` does
type mockOutboundSender struct {
	mock.Mock
}

func (m *mockOutboundSender) Extend(until time.Time) {
	m.Called(until)
}

func (m *mockOutboundSender) Shutdown(gentle bool) {
	m.Called(gentle)
}

func (m *mockOutboundSender) RetiredSince() time.Time {
	arguments := m.Called()
	return arguments.Get(0).(time.Time)
}

func (m *mockOutboundSender) QueueJSON(req CaduceusRequest, eventType, deviceID, transID string) {
	m.Called(req, eventType, deviceID, transID)
}

func (m *mockOutboundSender) QueueWrp(req CaduceusRequest, metaData map[string]string, eventType, deviceID, transID string) {
	m.Called(req, metaData, eventType, deviceID, transID)
}

// mockSenderWrapper needs to mock things that the `SenderWrapper` does
type mockSenderWrapper struct {
	mock.Mock
}

func (m *mockSenderWrapper) Update(list []webhook.W) {
	m.Called(list)
}

func (m *mockSenderWrapper) Queue(req CaduceusRequest) {
	m.Called(req)
}

func (m *mockSenderWrapper) Shutdown(gentle bool) {
	m.Called(gentle)
}

// mockServerProfiler needs to mock things that the `ServerProfiler` does
type mockServerProfiler struct {
	mock.Mock
}

func (m *mockServerProfiler) Send(inData interface{}) error {
	arguments := m.Called(inData)
	return arguments.Error(0)
}

func (m *mockServerProfiler) Report() (values []interface{}) {
	arguments := m.Called()
	if arguments.Get(0) == nil {
		return nil
	}

	return arguments.Get(0).([]interface{})
}

func (m *mockServerProfiler) Close() {
	m.Called()
}
