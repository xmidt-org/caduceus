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
	"io/ioutil"
	"net/http"
	"sync/atomic"

	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/wrp"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	uuid "github.com/satori/go.uuid"
)

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	log.Logger
	caduceusHandler          RequestHandler
	errorRequests            metrics.Counter
	emptyRequests            metrics.Counter
	invalidCount             metrics.Counter
	incomingQueueDepthMetric metrics.Gauge
	incomingQueueDepth       int64
	maxOutstanding           int64
}

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	debugLog := logging.Debug(sh.Logger)
	infoLog := logging.Info(sh.Logger)
	errorLog := logging.Error(sh.Logger)
	messageKey := logging.MessageKey()
	errorKey := logging.ErrorKey()

	infoLog.Log(messageKey, "Receiving incoming request...")

	outstanding := atomic.AddInt64(&sh.incomingQueueDepth, 1)
	defer atomic.AddInt64(&sh.incomingQueueDepth, -1)

	if 0 < sh.maxOutstanding && sh.maxOutstanding < outstanding {
		// return a 503
		response.WriteHeader(http.StatusServiceUnavailable)
		response.Write([]byte("Request placed on to queue.\n"))
		debugLog.Log(messageKey, "Request placed on to queue.\n")
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
	if err := decoder.Decode(msg); err != nil {
		// return a 400
		sh.invalidCount.Add(1.0)
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte("Invalid payload format.\n"))
		debugLog.Log(messageKey, "Invalid payload format.\n")
		return
	}

	sh.caduceusHandler.HandleRequest(0, fixWrp(msg))

	// return a 202
	response.WriteHeader(http.StatusAccepted)
	response.Write([]byte("Request placed on to queue.\n"))
	debugLog.Log(messageKey, "Request placed on to queue.")
}

func fixWrp(msg *wrp.Message) *wrp.Message {
	// "Fix" the WRP if needed.

	// Default to "application/json" if there is no content type, otherwise
	// use the one the source specified.
	if "" == msg.ContentType {
		msg.ContentType = "application/json"
	}

	// Ensure there is a transaction id even if we make one up
	if "" == msg.TransactionUUID {
		msg.TransactionUUID = uuid.NewV4().String()
	}

	return msg
}
