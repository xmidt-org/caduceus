/**
 * Copyright 2021 Comcast Cable Communications Management, LLC
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
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/metrics"
	uuid "github.com/satori/go.uuid"
	"github.com/xmidt-org/webpa-common/v2/logging"
	"github.com/xmidt-org/wrp-go/v3"
)

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	log.Logger
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

	logger := logging.GetLogger(request.Context())
	if logger == logging.DefaultLogger() {
		logger = sh.Logger
	}
	debugLog := level.Debug(logger)
	infoLog := level.Info(logger)
	errorLog := level.Error(logger)

	messageKey := logging.MessageKey()
	errorKey := logging.ErrorKey()

	infoLog.Log(messageKey, "Receiving incoming request...")

	if len(request.Header["Content-Type"]) != 1 || request.Header["Content-Type"][0] != "application/msgpack" {
		//return a 415
		response.WriteHeader(http.StatusUnsupportedMediaType)
		response.Write([]byte("Invalid Content-Type header(s). Expected application/msgpack. \n"))
		debugLog.Log(messageKey, "Invalid Content-Type header(s). Expected application/msgpack. \n")
		return
	}

	outstanding := atomic.AddInt64(&sh.incomingQueueDepth, 1)
	defer atomic.AddInt64(&sh.incomingQueueDepth, -1)

	if 0 < sh.maxOutstanding && sh.maxOutstanding < outstanding {
		// return a 503
		response.WriteHeader(http.StatusServiceUnavailable)
		response.Write([]byte("Incoming queue is full.\n"))
		debugLog.Log(messageKey, "Incoming queue is full.\n")
		return
	}

	sh.incomingQueueDepthMetric.Add(1.0)
	defer sh.incomingQueueDepthMetric.Add(-1.0)

	payload, err := ioutil.ReadAll(request.Body)
	if err != nil {
		sh.errorRequests.Add(1.0)
		errorLog.Log(messageKey, "Unable to retrieve the request body.", errorKey, err.Error)
		response.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(payload) == 0 {
		sh.emptyRequests.Add(1.0)
		errorLog.Log(messageKey, "Empty payload.", errorKey)
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
			debugLog.Log(messageKey, "Invalid payload format.")
		} else {
			response.Write([]byte("Invalid MessageType.\n"))
			debugLog.Log(messageKey, "Invalid MessageType.")
		}
		return
	}

	err = wrp.UTF8(msg)
	if err != nil {
		// return a 400
		sh.invalidCount.Add(1.0)
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte("Strings must be UTF-8.\n"))
		debugLog.Log(messageKey, "Strings must be UTF-8.")
		return
	}
	eventType = msg.FindEventStringSubMatch()

	sh.caduceusHandler.HandleRequest(0, sh.fixWrp(msg))

	// return a 202
	response.WriteHeader(http.StatusAccepted)
	response.Write([]byte("Request placed on to queue.\n"))

	debugLog.Log(messageKey, "event passed to senders.",
		"event", msg,
	)
}

func (sh *ServerHandler) recordQueueLatencyToHistogram(startTime time.Time, eventType string) {
	endTime := sh.now()
	sh.incomingQueueLatency.With("event", eventType).Observe(endTime.Sub(startTime).Seconds())
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
		sh.modifiedWRPCount.With("reason", reason).Add(1.0)
	}

	return msg
}
