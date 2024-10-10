// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package mocks

import (
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
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

func (m *Wrapper) Queue(msg *wrp.Message) {
	m.Called(msg)
}

func (m *Wrapper) Shutdown(gentle bool) {
	m.Called(gentle)
}

func (m *Wrapper) Update(r []ancla.Register) {
	m.Called(r)
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
func (m *Counter) With(labelValues ...string) prometheus.Counter {
	for _, v := range labelValues {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	args := m.Called(labelValues)
	return args.Get(0).(prometheus.Counter)
}
