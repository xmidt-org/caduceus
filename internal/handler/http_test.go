// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package handler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/caduceus/internal/handler"
	"github.com/xmidt-org/caduceus/internal/mocks"
	"go.uber.org/zap/zaptest"
)

func TestNew(t *testing.T) {
	logger := zaptest.NewLogger(t)
	maxOutstanding := int64(10)
	incomingQueueDepth := int64(100)

	wrapper := new(mocks.Wrapper)
	telemetry := new(handler.Telemetry)

	sh, err := handler.New(wrapper, logger, telemetry, maxOutstanding, incomingQueueDepth)

	assert.NotNil(t, sh)
	assert.NoError(t, err)
}
