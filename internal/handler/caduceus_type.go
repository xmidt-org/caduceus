// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package handler

import (
	"go.uber.org/zap"

	"github.com/xmidt-org/caduceus/internal/sink"
	"github.com/xmidt-org/wrp-go/v3"
)

type RequestHandler interface {
	HandleRequest(workerID int, msg *wrp.Message)
}

type CaduceusHandler struct {
	wrapper sink.Wrapper
	Logger  *zap.Logger
}

func (ch *CaduceusHandler) HandleRequest(workerID int, msg *wrp.Message) {
	ch.Logger.Info("Worker received a request, now passing to sender", zap.Int("workerId", workerID))
	ch.wrapper.Queue(msg)
}
