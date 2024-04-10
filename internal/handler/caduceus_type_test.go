// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package handler

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/caduceus/internal/mocks"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap/zaptest"
)

func TestCaduceusHandler(t *testing.T) {
	logger := zaptest.NewLogger(t)

	fakeSenderWrapper := new(mocks.MockSinkWrapper)
	fakeSenderWrapper.On("Queue", mock.AnythingOfType("*wrp.Message")).Return().Once()

	testHandler := CaduceusHandler{
		wrapper: fakeSenderWrapper,
		Logger:  logger,
	}

	t.Run("TestHandleRequest", func(t *testing.T) {
		testHandler.HandleRequest(0, &wrp.Message{})

		fakeSenderWrapper.AssertExpectations(t)
	})
}
