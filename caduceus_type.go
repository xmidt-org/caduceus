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
