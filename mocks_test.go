// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"time"
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

// mockHistogram provides the mock implementation of the metrics.Histogram object
type mockHistogram struct {
	mock.Mock
}

func (m *mockHistogram) Observe(value float64) {
	m.Called(value)
}

func (m *mockHistogram) With(labelValues ...string) metrics.Histogram {
	for _, v := range labelValues {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	m.Called(labelValues)
	return m
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

func (m *mockCaduceusMetricsRegistry) NewHistogram(name string, buckets int) metrics.Histogram {
	args := m.Called(name)
	return args.Get(0).(metrics.Histogram)
}

// mockTime provides two mock time values
func mockTime(one, two time.Time) func() time.Time {
	var called bool
	return func() time.Time {
		if called {
			return two
		}
		called = true
		return one
	}
}
