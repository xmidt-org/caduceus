// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
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

// func (m *mockSenderWrapper) Update(list []ancla.InternalWebhook) {
// 	m.Called(list)
// }

func (m *mockSenderWrapper) Queue(msg *wrp.Message) {
	m.Called(msg)
}

func (m *mockSenderWrapper) Shutdown(gentle bool) {
	m.Called(gentle)
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

type mockCounter struct {
	mock.Mock
}

func (m *mockCounter) Add(delta float64) {
	m.Called(delta)
}

func (m *mockCounter) Inc (){
	m.Called(1)
}
func (m *mockCounter) With(labelValues ...string) prometheus.Counter {
	for _, v := range labelValues {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	args := m.Called(labelValues)
	return args.Get(0).(prometheus.Counter)
}
