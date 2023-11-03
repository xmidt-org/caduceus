// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"time"

	"go.uber.org/zap"

	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/ancla"

	"github.com/xmidt-org/wrp-go/v3"
)

// Below is the struct we're using to contain the data from a provided config file
// TODO: Try to figure out how to make bucket ranges configurable
type CaduceusConfig struct {
	AuthHeader       []string
	NumWorkerThreads int
	JobQueueSize     int
	Sender           SenderConfig
	JWTValidators    []JWTValidator
	Webhook          ancla.Config
	Listener         ancla.ListenerConfig
	AllowInsecureTLS bool
}

type SenderConfig struct {
	NumWorkersPerSender             int
	QueueSizePerSender              int
	CutOffPeriod                    time.Duration
	Linger                          time.Duration
	ClientTimeout                   time.Duration
	DisableClientHostnameValidation bool
	ResponseHeaderTimeout           time.Duration
	IdleConnTimeout                 time.Duration
	DeliveryRetries                 int
	DeliveryInterval                time.Duration
	CustomPIDs                      []string
	DisablePartnerIDs               bool
}

type CaduceusMetricsRegistry interface {
	NewCounter(name string) metrics.Counter
	NewGauge(name string) metrics.Gauge
	NewHistogram(name string, buckets int) metrics.Histogram
}

type RequestHandler interface {
	HandleRequest(workerID int, msg *wrp.Message)
}

type CaduceusHandler struct {
	senderWrapper SenderWrapper
	*zap.Logger
}

func (ch *CaduceusHandler) HandleRequest(workerID int, msg *wrp.Message) {
	ch.Logger.Info("Worker received a request, now passing to sender", zap.Int("workerId", workerID))
	ch.senderWrapper.Queue(msg)
}
