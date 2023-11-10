// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"

	"github.com/stretchr/testify/mock"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/adapter"
	"github.com/xmidt-org/wrp-go/v3"
)

func TestCaduceusHandler(t *testing.T) {
	logger := adapter.DefaultLogger().Logger

	fakeSenderWrapper := new(mockSenderWrapper)
	fakeSenderWrapper.On("Queue", mock.AnythingOfType("*wrp.Message")).Return().Once()

	testHandler := CaduceusHandler{
		senderWrapper: fakeSenderWrapper,
		Logger:        logger,
	}

	t.Run("TestHandleRequest", func(t *testing.T) {
		testHandler.HandleRequest(0, &wrp.Message{})

		fakeSenderWrapper.AssertExpectations(t)
	})
}
