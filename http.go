// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/go-kit/kit/metrics"
	uuid "github.com/satori/go.uuid"

	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/webpa-common/v2/adapter"
	"github.com/xmidt-org/wrp-go/v3"
)

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	*zap.Logger
	caduceusHandler          RequestHandler
	errorRequests            metrics.Counter
	emptyRequests            metrics.Counter
	invalidCount             metrics.Counter
	incomingQueueDepthMetric metrics.Gauge
	modifiedWRPCount         metrics.Counter
	incomingQueueDepth       int64
	maxOutstanding           int64
	incomingQueueLatency     metrics.Histogram
	now                      func() time.Time
}

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	eventType := unknownEventType
	// find time difference, add to metric after function finishes
	defer func(s time.Time) {
		sh.recordQueueLatencyToHistogram(s, eventType)
	}(sh.now())

	logger := sallust.Get(request.Context())
	if logger == adapter.DefaultLogger().Logger {
		logger = sh.Logger
	}

	logger.Info("Receiving incoming request...")

	if len(request.Header["Content-Type"]) != 1 || request.Header["Content-Type"][0] != "application/msgpack" {
		//return a 415
		response.WriteHeader(http.StatusUnsupportedMediaType)
		response.Write([]byte("Invalid Content-Type header(s). Expected application/msgpack. \n"))
		logger.Debug("Invalid Content-Type header(s). Expected application/msgpack. \n")
		return
	}

	outstanding := atomic.AddInt64(&sh.incomingQueueDepth, 1)
	defer atomic.AddInt64(&sh.incomingQueueDepth, -1)

	if 0 < sh.maxOutstanding && sh.maxOutstanding < outstanding {
		// return a 503
		response.WriteHeader(http.StatusServiceUnavailable)
		response.Write([]byte("Incoming queue is full.\n"))
		logger.Debug("Incoming queue is full.\n")
		return
	}

	sh.incomingQueueDepthMetric.Add(1.0)
	defer sh.incomingQueueDepthMetric.Add(-1.0)

	payload, err := io.ReadAll(request.Body)
	if err != nil {
		sh.errorRequests.Add(1.0)
		logger.Error("Unable to retrieve the request body.", zap.Error(err))
		response.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(payload) == 0 {
		sh.emptyRequests.Add(1.0)
		logger.Error("Empty payload.")
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte("Empty payload.\n"))
		return
	}

	decoder := wrp.NewDecoderBytes(payload, wrp.Msgpack)
	msg := new(wrp.Message)

	err = decoder.Decode(msg)
	if err != nil || msg.MessageType() != 4 {
		// return a 400
		sh.invalidCount.Add(1.0)
		response.WriteHeader(http.StatusBadRequest)
		if err != nil {
			response.Write([]byte("Invalid payload format.\n"))
			logger.Debug("Invalid payload format.")
		} else {
			response.Write([]byte("Invalid MessageType.\n"))
			logger.Debug("Invalid MessageType.")
		}
		return
	}

	err = wrp.UTF8(msg)
	if err != nil {
		// return a 400
		sh.invalidCount.Add(1.0)
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte("Strings must be UTF-8.\n"))
		logger.Debug("Strings must be UTF-8.")
		return
	}
	eventType = msg.FindEventStringSubMatch()

	sh.caduceusHandler.HandleRequest(0, sh.fixWrp(msg))

	// return a 202
	response.WriteHeader(http.StatusAccepted)
	response.Write([]byte("Request placed on to queue.\n"))

	logger.Debug("event passed to senders.", zap.Any("event", msg))
}

func (sh *ServerHandler) recordQueueLatencyToHistogram(startTime time.Time, eventType string) {
	endTime := sh.now()
	sh.incomingQueueLatency.With(eventLabel, eventType).Observe(endTime.Sub(startTime).Seconds())
}

func (sh *ServerHandler) fixWrp(msg *wrp.Message) *wrp.Message {
	// "Fix" the WRP if needed.
	var reason string

	// Default to "application/json" if there is no content type, otherwise
	// use the one the source specified.
	if "" == msg.ContentType {
		msg.ContentType = wrp.MimeTypeJson
		reason = emptyContentTypeReason
	}

	// Ensure there is a transaction id even if we make one up
	if "" == msg.TransactionUUID {
		msg.TransactionUUID = uuid.NewV4().String()
		if reason == "" {
			reason = emptyUUIDReason
		} else {
			reason = bothEmptyReason
		}
	}

	if reason != "" {
		sh.modifiedWRPCount.With(reasonLabel, reason).Add(1.0)
	}

	return msg
}
