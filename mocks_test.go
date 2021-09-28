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
	"unicode/utf8"

	"github.com/go-kit/kit/metrics"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/wrp-go/v3"
)

// mockHandler only needs to mock the `HandleRequest` method
type mockHandler struct {
	mock.Mock
}

func (m *mockHandler) HandleRequest(workerID int, msg *wrp.Message) {
	m.Called(workerID, msg)
}

// mockSenderWrapper needs to mock things that the `SenderWrapper` does
type mockSenderWrapper struct {
	mock.Mock
}

func (m *mockSenderWrapper) Update(list []ancla.InternalWebhook) {
	m.Called(list)
}

func (m *mockSenderWrapper) Queue(msg *wrp.Message) {
	m.Called(msg)
}

func (m *mockSenderWrapper) Shutdown(gentle bool) {
	m.Called(gentle)
}

// mockCounter provides the mock implementation of the metrics.Counter object
type mockCounter struct {
	mock.Mock
}

func (m *mockCounter) Add(delta float64) {
	m.Called(delta)
}

func (m *mockCounter) With(labelValues ...string) metrics.Counter {
	for _, v := range labelValues {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	args := m.Called(labelValues)
	return args.Get(0).(metrics.Counter)
}

// mockGauge provides the mock implementation of the metrics.Counter object
type mockGauge struct {
	mock.Mock
}

func (m *mockGauge) Add(delta float64) {
	m.Called(delta)
}

func (m *mockGauge) Set(value float64) {
	// We're setting time values & the ROI seems pretty low with this level
	// of validation...
	//m.Called(value)
}

func (m *mockGauge) With(labelValues ...string) metrics.Gauge {
	for _, v := range labelValues {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	args := m.Called(labelValues)
	return args.Get(0).(metrics.Gauge)
}

// mockCaduceusMetricsRegistry provides the mock implementation of the
// CaduceusMetricsRegistry  object
type mockCaduceusMetricsRegistry struct {
	mock.Mock
}

func (m *mockCaduceusMetricsRegistry) NewCounter(name string) metrics.Counter {
	args := m.Called(name)
	return args.Get(0).(metrics.Counter)
}

func (m *mockCaduceusMetricsRegistry) NewGauge(name string) metrics.Gauge {
	args := m.Called(name)
	return args.Get(0).(metrics.Gauge)
}
