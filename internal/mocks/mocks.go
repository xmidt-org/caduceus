// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package mocks

import (
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/caduceus/internal/handler"
	"github.com/xmidt-org/caduceus/internal/sink"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

// mockHandler only needs to mock the `HandleRequest` method
type Handler struct {
	mock.Mock

	SinkWrapper        sink.Wrapper
	Logger             *zap.Logger
	Telemetry          *handler.Telemetry
	IncomingQueueDepth int64
	MaxOutstanding     int64
	Now                func() time.Time
}

func (m *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	m.Called(r)
}

// mockSenderWrapper needs to mock things that the `SenderWrapper` does
type Wrapper struct {
	mock.Mock
}

func (m *Wrapper) Update(list []ancla.Register) {
	m.Called(list)
}

func (m *Wrapper) Queue(msg *wrp.Message) {
	m.Called(msg)
}

func (m *Wrapper) Shutdown(gentle bool) {
	m.Called(gentle)
}

// mockTime provides two mock time values
func Time(one, two time.Time) func() time.Time {
	var called bool
	return func() time.Time {
		if called {
			return two
		}
		called = true
		return one
	}
}

type Counter struct {
	mock.Mock
}

func (m *Counter) Add(delta float64) {
	m.Called(delta)
}

func (m *Counter) Inc() {
	m.Called(1)
}

func (m *Counter) Describe(ch chan<- *prometheus.Desc) {
	m.Called(ch)
}

func (m *Counter) Collect(ch chan<- prometheus.Metric) {
	m.Called(ch)
}

func (m *Counter) Desc() *prometheus.Desc {
	args := m.Called()
	return args.Get(0).(*prometheus.Desc)
}

func (m *Counter) Write(metric *dto.Metric) error {
	args := m.Called(metric)
	return args.Error(0)
}

func (m *Counter) With(labelValues ...string) prometheus.Counter {
	for _, v := range labelValues {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	args := m.Called(labelValues)
	return args.Get(0).(prometheus.Counter)
}

type Gauge struct {
	mock.Mock
}

func (m *Gauge) Set(num float64) {
	m.Called(num)
}

func (m *Gauge) Inc() {
	m.Called(1)
}

func (m *Gauge) Dec() {
	m.Called(-1)
}

func (m *Gauge) Add(num float64) {
	m.Called(num)
}

func (m *Gauge) Sub(num float64) {
	m.Called(num)
}

func (m *Gauge) SetToCurrentTime() {
	m.Called()
}

func (m *Gauge) Describe(ch chan<- *prometheus.Desc) {
	m.Called(ch)
}

func (m *Gauge) Collect(ch chan<- prometheus.Metric) {
	m.Called(ch)
}

func (m *Gauge) Desc() *prometheus.Desc {
	args := m.Called()
	return args.Get(0).(*prometheus.Desc)
}

func (m *Gauge) Write(metric *dto.Metric) error {
	args := m.Called(metric)
	return args.Get(0).(error)
}

type Histogram struct {
	mock.Mock
}

func (m *Histogram) GetMetricWith(labels prometheus.Labels) (prometheus.Observer, error) {
	args := m.Called(labels)
	return args.Get(0).(prometheus.Observer), args.Error(1)
}

func (m *Histogram) GetMetricWithLabelValues(lvs ...string) (prometheus.Observer, error) {
	args := m.Called(lvs)
	return args.Get(0).(prometheus.Observer), args.Error(1)
}

func (m *Histogram) With(labels prometheus.Labels) prometheus.Observer {
	args := m.Called(labels)
	return args.Get(0).(prometheus.Observer)
}

func (m *Histogram) WithLabelValues(lvs ...string) prometheus.Observer {
	for _, v := range lvs {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	args := m.Called(lvs)
	return args.Get(0).(prometheus.Observer)
}

func (m *Histogram) CurryWith(labels prometheus.Labels) (prometheus.ObserverVec, error) {
	args := m.Called(labels)
	return args.Get(0).(prometheus.ObserverVec), args.Error(1)
}

func (m *Histogram) MustCurryWith(labels prometheus.Labels) prometheus.ObserverVec {
	args := m.Called(labels)
	return args.Get(0).(prometheus.ObserverVec)
}

func (m *Histogram) Describe(ch chan<- *prometheus.Desc) {
	m.Called(ch)
}

func (m *Histogram) Collect(ch chan<- prometheus.Metric) {
	m.Called(ch)
}
