/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/go-kit/kit/metrics"
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

// mockCounter provides the mock implementation of the metrics.Counter object
type mockCounter struct {
	mock.Mock
}

func (m *mockCounter) Add(delta float64) {
	m.Called(delta)
}

func (m *mockCounter) With(labelValues ...string) metrics.Counter {
	m.Called(labelValues)
	return nil
}
