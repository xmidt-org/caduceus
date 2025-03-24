// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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

// metric mocks

type mockCollector struct {
}

func (m *mockCollector) Describe(chan<- *prometheus.Desc) {
}
func (m *mockCollector) Collect(chan<- prometheus.Metric) {
}

type mockMetric struct {
}

func (m *mockMetric) Desc() *prometheus.Desc {
	return &prometheus.Desc{}
}

func (m *mockMetric) Write(*dto.Metric) error {
	return nil
}

// mockCounter provides the mock implementation of the metrics.Counter object
type mockCounter struct {
	mockCollector
	mockMetric
	mock.Mock
}

func (m *mockCounter) Add(delta float64) {
	m.Called(delta)
}

func (m *mockCounter) Inc() { m.Called() }

func (m *mockCounter) With(labels prometheus.Labels) prometheus.Counter {
	for _, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}

	m.Called(labels)
	return m
}

func (m *mockCounter) CurryWith(labels prometheus.Labels) (*prometheus.CounterVec, error) {
	m.Called(labels)
	labelnames := []string{}
	for l, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
		labelnames = append(labelnames, l)
	}

	return prometheus.NewCounterVec(prometheus.CounterOpts{}, labelnames).CurryWith(labels)
}

func (m *mockCounter) GetMetricWith(labels prometheus.Labels) (prometheus.Counter, error) {
	m.Called(labels)
	return m, nil
}

func (m *mockCounter) GetMetricWithLabelValues(lvs ...string) (prometheus.Counter, error) {
	m.Called(lvs)
	return m, nil
}

func (m *mockCounter) MustCurryWith(labels prometheus.Labels) *prometheus.CounterVec {
	m.Called(labels)
	labelnames := []string{}
	for l, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
		labelnames = append(labelnames, l)
	}

	return prometheus.NewCounterVec(prometheus.CounterOpts{}, labelnames).MustCurryWith(labels)
}

func (m *mockCounter) WithLabelValues(lvs ...string) prometheus.Counter {
	m.Called(lvs)
	return m
}

// mockGauge provides the mock implementation of the metrics.Counter object
type mockGauge struct {
	mockCollector
	mockMetric
	mock.Mock
}

func (m *mockGauge) CurryWith(labels prometheus.Labels) (*prometheus.GaugeVec, error) {
	m.Called(labels)
	labelnames := []string{}
	for l, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
		labelnames = append(labelnames, l)
	}

	return prometheus.NewGaugeVec(prometheus.GaugeOpts{}, labelnames).CurryWith(labels)
}

func (m *mockGauge) MustCurryWith(labels prometheus.Labels) *prometheus.GaugeVec {
	m.Called(labels)
	labelnames := []string{}
	for l, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
		labelnames = append(labelnames, l)
	}

	return prometheus.NewGaugeVec(prometheus.GaugeOpts{}, labelnames).MustCurryWith(labels)
}

func (m *mockGauge) GetMetricWith(labels prometheus.Labels) (prometheus.Gauge, error) {
	m.Called(labels)
	return m, nil
}

func (m *mockGauge) GetMetricWithLabelValues(lvs ...string) (prometheus.Gauge, error) {
	m.Called(lvs)
	return m, nil
}

func (m *mockGauge) WithLabelValues(lvs ...string) prometheus.Gauge {
	m.Called(lvs)
	return m
}

func (m *mockGauge) With(labels prometheus.Labels) prometheus.Gauge {
	for _, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}

	m.Called(labels)
	return m
}

func (m *mockGauge) Add(delta float64) {
	m.Called(delta)
}

func (m *mockGauge) Set(value float64) {
	m.Called(value)
}

func (m *mockGauge) Inc() { m.Called() }

func (m *mockGauge) Dec() { m.Called() }

func (m *mockGauge) Sub(val float64) {
	m.Called(val)
}

func (m *mockGauge) SetToCurrentTime() { m.Called() }

// mockHistogram provides the mock implementation of the metrics.Histogram object
type mockHistogram struct {
	mockCollector
	mock.Mock
}

func (m *mockHistogram) Observe(value float64) {
	m.Called(value)
}

func (m *mockHistogram) With(labels prometheus.Labels) prometheus.Observer {
	for _, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}

	m.Called(labels)
	return m
}

func (m *mockHistogram) CurryWith(labels prometheus.Labels) (prometheus.ObserverVec, error) {
	m.Called(labels)
	return m, nil
}

func (m *mockHistogram) GetMetricWith(labels prometheus.Labels) (prometheus.Observer, error) {
	m.Called(labels)
	return m, nil
}

func (m *mockHistogram) GetMetricWithLabelValues(lvs ...string) (prometheus.Observer, error) {
	m.Called(lvs)
	return m, nil
}

func (m *mockHistogram) MustCurryWith(labels prometheus.Labels) prometheus.ObserverVec {
	m.Called(labels)
	return m
}

func (m *mockHistogram) WithLabelValues(lvs ...string) prometheus.Observer {
	m.Called(lvs)
	return m
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
