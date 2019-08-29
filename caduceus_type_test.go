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
	"testing"

	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/wrp"
	"github.com/stretchr/testify/mock"
)

func TestCaduceusHandler(t *testing.T) {
	logger := logging.DefaultLogger()

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
