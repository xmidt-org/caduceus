// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package handler

import (
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/caduceus/internal/sink"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

// mockHandler only needs to mock the `HandleRequest` method
type mockHandler struct {
	mock.Mock

	SinkWrapper sink.Wrapper
	Logger      *zap.Logger
}

func (m *mockHandler) handleRequest(msg *wrp.Message) {
	m.Called(msg)
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
