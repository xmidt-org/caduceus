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
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/Comcast/webpa-common/wrp/wrphttp"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/satori/go.uuid"
)

// failureText is human readable text for the failure message
const failureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

var (
	// eventPattern is the precompiled regex that selects the top level event
	// classifier
	eventPattern = regexp.MustCompile(`^event:(?P<event>[^/]+)`)
)

// outboundRequest stores the outgoing request and assorted data that has been
// collected so far (to save on processing the information again).
type outboundRequest struct {
	req         CaduceusRequest
	event       string
	transID     string
	deviceID    string
	contentType string
}

// FailureMessage is a helper that lets us easily create a json struct to send
// when we have to cut and endpoint off.
type FailureMessage struct {
	Text         string    `json:"text"`
	Original     webhook.W `json:"webhook_registration"`
	CutOffPeriod string    `json:"cut_off_period"`
	QueueSize    int       `json:"queue_size"`
	Workers      int       `json:"worker_count"`
}

// OutboundSenderFactory is a configurable factory for OutboundSender objects.
type OutboundSenderFactory struct {
	// The WebHookListener to service
	Listener webhook.W

	// The http client to use for requests.
	Client *http.Client

	// The number of delivery workers to create and use.
	NumWorkers int

	// The queue depth to buffer events before we declare overflow, shut
	// off the message delivery, and basically put the endpoint in "timeout."
	QueueSize int

	// The amount of time to cut off the consumer if they don't keep up.
	// Must be greater then 0 seconds
	CutOffPeriod time.Duration

	// Metrics registry.
	MetricsRegistry CaduceusMetricsRegistry

	// The logger to use.
	Logger log.Logger
}

type OutboundSender interface {
	Extend(time.Time)
	Shutdown(bool)
	RetiredSince() time.Time
	QueueJSON(CaduceusRequest, string, string, string)
	QueueWrp(CaduceusRequest)
}

// CaduceusOutboundSender is the outbound sender object.
type CaduceusOutboundSender struct {
	listener                 webhook.W
	deliverUntil             time.Time
	dropUntil                time.Time
	client                   *http.Client
	secret                   []byte
	events                   []*regexp.Regexp
	matcher                  []*regexp.Regexp
	queueSize                int
	queue                    chan outboundRequest
	deliveryCounter          metrics.Counter
	droppedQueueFullCounter  metrics.Counter
	droppedExpiredCounter    metrics.Counter
	droppedNetworkErrCounter metrics.Counter
	cutOffCounter            metrics.Counter
	queueDepthGauge          metrics.Gauge
	wg                       sync.WaitGroup
	cutOffPeriod             time.Duration
	failureMsg               FailureMessage
	logger                   log.Logger
	mutex                    sync.RWMutex
}

// New creates a new OutboundSender object from the factory, or returns an error.
func (osf OutboundSenderFactory) New() (obs OutboundSender, err error) {
	if _, err = url.ParseRequestURI(osf.Listener.Config.URL); nil != err {
		return
	}

	if nil == osf.Client {
		err = errors.New("nil http.Client")
		return
	}

	if 0 == osf.CutOffPeriod.Nanoseconds() {
		err = errors.New("Invalid CutOffPeriod")
		return
	}

	if nil == osf.Logger {
		err = errors.New("Logger required")
		return
	}

	caduceusOutboundSender := &CaduceusOutboundSender{
		listener:     osf.Listener,
		client:       osf.Client,
		queueSize:    osf.QueueSize,
		cutOffPeriod: osf.CutOffPeriod,
		deliverUntil: osf.Listener.Until,
		logger:       osf.Logger,
		failureMsg: FailureMessage{
			Original:     osf.Listener,
			Text:         failureText,
			CutOffPeriod: osf.CutOffPeriod.String(),
			QueueSize:    osf.QueueSize,
			Workers:      osf.NumWorkers,
		},
	}

	caduceusOutboundSender.deliveryCounter = osf.MetricsRegistry.NewCounter(DeliveryCounter)

	caduceusOutboundSender.cutOffCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerCounter).With("url", osf.Listener.Config.URL)

	caduceusOutboundSender.droppedQueueFullCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerDroppedMsgCounter).With("url", osf.Listener.Config.URL, "reason", "queue_full")

	caduceusOutboundSender.droppedExpiredCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerDroppedMsgCounter).With("url", osf.Listener.Config.URL, "reason", "expired")

	caduceusOutboundSender.droppedNetworkErrCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerDroppedMsgCounter).With("url", osf.Listener.Config.URL, "reason", "network_err")

	caduceusOutboundSender.queueDepthGauge = osf.MetricsRegistry.
		NewGauge(OutgoingQueueDepth).With("url", osf.Listener.Config.URL)

	// Don't share the secret with others when there is an error.
	caduceusOutboundSender.failureMsg.Original.Config.Secret = "XxxxxX"

	if "" != osf.Listener.Config.Secret {
		caduceusOutboundSender.secret = []byte(osf.Listener.Config.Secret)
	}

	if "" != osf.Listener.FailureURL {
		if _, err = url.ParseRequestURI(osf.Listener.FailureURL); nil != err {
			return
		}
	}

	// Give us some head room so that we don't block when we get near the
	// completely full point.
	caduceusOutboundSender.queue = make(chan outboundRequest, osf.QueueSize)

	// Create the event regex objects
	for _, event := range osf.Listener.Events {
		var re *regexp.Regexp
		if re, err = regexp.Compile(event); nil != err {
			return
		}

		caduceusOutboundSender.events = append(caduceusOutboundSender.events, re)
	}
	if nil == caduceusOutboundSender.events {
		err = errors.New("Events must not be empty.")
		return
	}

	// Create the matcher regex objects
	for _, item := range osf.Listener.Matcher.DeviceId {
		if ".*" == item {
			// Match everything - skip the filtering
			caduceusOutboundSender.matcher = nil
			break
		}

		var re *regexp.Regexp
		if re, err = regexp.Compile(item); nil != err {
			err = fmt.Errorf("Invalid matcher item: '%s'", item)
			return
		}
		caduceusOutboundSender.matcher = append(caduceusOutboundSender.matcher, re)
	}

	caduceusOutboundSender.wg.Add(osf.NumWorkers)
	for i := 0; i < osf.NumWorkers; i++ {
		go caduceusOutboundSender.worker(i)
	}

	obs = caduceusOutboundSender
	return
}

// Extend extends the time the CaduceusOutboundSender will deliver messages to the
// specified time.  The new delivery cutoff time must be after the previously
// set delivery cutoff time.
func (obs *CaduceusOutboundSender) Extend(until time.Time) {

	obs.mutex.Lock()
	if until.After(obs.deliverUntil) {
		obs.deliverUntil = until
	}
	obs.mutex.Unlock()
}

// Shutdown causes the CaduceusOutboundSender to stop it's activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (obs *CaduceusOutboundSender) Shutdown(gentle bool) {
	close(obs.queue)
	obs.mutex.Lock()
	if false == gentle {
		obs.deliverUntil = time.Time{}
	}
	obs.mutex.Unlock()
	obs.wg.Wait()

	obs.mutex.Lock()
	obs.deliverUntil = time.Time{}
	obs.mutex.Unlock()
}

// RetiredSince returns the time the CaduceusOutboundSender retired (which could be in
// the future).
func (obs *CaduceusOutboundSender) RetiredSince() time.Time {
	obs.mutex.RLock()
	deliverUntil := obs.deliverUntil
	obs.mutex.RUnlock()

	return deliverUntil
}

// QueueJSON is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
func (obs *CaduceusOutboundSender) QueueJSON(req CaduceusRequest,
	eventType, deviceID, transID string) {

	obs.mutex.RLock()
	deliverUntil := obs.deliverUntil
	dropUntil := obs.dropUntil
	obs.mutex.RUnlock()

	now := time.Now()

	if now.Before(deliverUntil) && now.After(dropUntil) {
		for _, eventRegex := range obs.events {
			if eventRegex.MatchString(eventType) {
				matchDevice := (nil == obs.matcher)
				if nil != obs.matcher {
					for _, deviceRegex := range obs.matcher {
						if deviceRegex.MatchString(deviceID) {
							matchDevice = true
							break
						}
					}
				}
				if matchDevice {
					if len(obs.queue) < obs.queueSize {
						req.OutgoingPayload = req.RawPayload
						outboundReq := outboundRequest{
							req:         req,
							event:       eventType,
							transID:     transID,
							deviceID:    deviceID,
							contentType: "application/json",
						}
						logging.Debug(obs.logger).Log(logging.MessageKey(), "JSON Sent to obs queue", "url",
							obs.listener.Config.URL)
						obs.queueDepthGauge.Add(1.0)
						obs.queue <- outboundReq
					} else {
						obs.droppedQueueFullCounter.Add(1.0)
						obs.queueOverflow()
					}
				}
			}
		}
	}
}

// QueueWrp is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
func (obs *CaduceusOutboundSender) QueueWrp(req CaduceusRequest) {
	obs.mutex.RLock()
	deliverUntil := obs.deliverUntil
	dropUntil := obs.dropUntil
	obs.mutex.RUnlock()

	now := time.Now()

	var debugLog = logging.Debug(obs.logger)

	if now.Before(deliverUntil) && now.After(dropUntil) {
		for _, eventRegex := range obs.events {
			if eventRegex.MatchString(req.PayloadAsWrp.Destination) {
				matchDevice := (nil == obs.matcher)
				if nil != obs.matcher {
					for _, deviceRegex := range obs.matcher {
						if deviceRegex.MatchString(req.PayloadAsWrp.Source) {
							matchDevice = true
							break
						}
					}
				}
				/*
					// if the device id matches then we want to look through all the metadata
					// and make sure that the obs metadata matches the metadata provided
					if matchDevice {
						for key, val := range metaData {
							if matchers, ok := obs.matcher[key]; ok {
								for _, deviceRegex := range matchers {
									matchDevice = false
									if deviceRegex.MatchString(val) {
										matchDevice = true
										break
									}
								}

								// metadata was provided but did not match our expectations,
								// so it is time to drop the message
								if !matchDevice {
									break
								}
							}
						}
					}
				*/
				if matchDevice {
					if len(obs.queue) < obs.queueSize {
						// TODO we should break this code out into a function set a object
						// creation time & choose if we should send the raw WRP message
						// or if we should send just the payload + headers in the "HTTP"
						// form.

						// TODO we should really transcode from WRP into an HTTP format if
						// asked for, otherwise we should deliver the WRP in that form.
						req.OutgoingPayload = req.PayloadAsWrp.Payload

						// Default to "application/json" if there is no content type, otherwise
						// use the one the source specified.
						ct := req.PayloadAsWrp.ContentType
						if "" == ct {
							ct = "application/json"
						}

						match := eventPattern.FindStringSubmatch(req.PayloadAsWrp.Destination)
						eventType := "unknown"
						if match != nil {
							eventType = match[1]
						}

						outboundReq := outboundRequest{req: req,
							event:       eventType,
							transID:     req.PayloadAsWrp.TransactionUUID,
							deviceID:    req.PayloadAsWrp.Source,
							contentType: ct,
						}
						obs.queueDepthGauge.Add(1.0)
						obs.queue <- outboundReq
						debugLog.Log(logging.MessageKey(), "WRP Sent to obs queue", "url", obs.listener.Config.URL)
					} else {
						obs.queueOverflow()
						obs.droppedQueueFullCounter.Add(1.0)
					}
				}
			} else {
				debugLog.Log(logging.MessageKey(),
					fmt.Sprintf("Regex did not match. got != expected: '%s' != '%s'\n",
						req.PayloadAsWrp.Destination, eventRegex.String()))
			}
		}
	} else {
		debugLog.Log(logging.MessageKey(), "Outside delivery window")
		debugLog.Log("now", now, "before", deliverUntil, "after", dropUntil)
		obs.droppedExpiredCounter.Add(1.0)
	}
}

// helper function to get the right delivery counter to increment
func (obs *CaduceusOutboundSender) getDeliveryCounter(status int) metrics.Counter {
	if -1 == status {
		return obs.deliveryCounter.With("url", obs.listener.Config.URL, "code", "failure")
	}

	s := strconv.Itoa(status)
	return obs.deliveryCounter.With("url", obs.listener.Config.URL, "code", s)
}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (obs *CaduceusOutboundSender) worker(id int) {
	defer obs.wg.Done()

	// Make a local copy of the hmac
	var h hash.Hash

	// Create the base sha1 hash object for each thread
	if nil != obs.secret {
		h = hmac.New(sha1.New, obs.secret)
	}

	// Only optimize the successful answers
	delivered200 := obs.getDeliveryCounter(200)
	delivered201 := obs.getDeliveryCounter(201)
	delivered202 := obs.getDeliveryCounter(202)
	delivered204 := obs.getDeliveryCounter(204)

	for work := range obs.queue {
		obs.queueDepthGauge.Add(-1.0)
		obs.mutex.RLock()
		deliverUntil := obs.deliverUntil
		dropUntil := obs.dropUntil
		obs.mutex.RUnlock()

		now := time.Now()
		if now.Before(deliverUntil) && now.After(dropUntil) {
			payload := work.req.OutgoingPayload
			payloadReader := bytes.NewReader(payload)
			req, err := http.NewRequest("POST", obs.listener.Config.URL, payloadReader)
			if nil != err {
				// Report drop
				obs.droppedNetworkErrCounter.Add(1.0)
				logging.Error(obs.logger).Log(logging.MessageKey(), "New Post request", "url", obs.listener.Config.URL,
					logging.ErrorKey(), err)
			} else {

				req.Header.Set("Content-Type", work.contentType)

				// Add x-Midt-* headers if possible
				if nil != work.req.PayloadAsWrp {
					wrphttp.AddMessageHeaders(req.Header, work.req.PayloadAsWrp)
				}

				// Provide the old headers for now
				req.Header.Set("X-Webpa-Event", work.event)

				// Ensure there is a transaction id even if we make one up
				tid := work.transID
				if "" == tid {
					tid = uuid.NewV4().String()
				}
				req.Header.Set("X-Webpa-Transaction-Id", tid)

				// Add the device id without the trailing service
				id, _ := device.ParseID(work.deviceID)
				req.Header.Set("X-Webpa-Device-Id", string(id))
				req.Header.Set("X-Webpa-Device-Name", string(id))

				if nil != h {
					h.Reset()
					h.Write(payload)
					sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
					req.Header.Set("X-Webpa-Signature", sig)
				}

				// Send it
				resp, err := obs.client.Do(req)
				if nil != err {
					// Report failure
					obs.getDeliveryCounter(-1).With("event", work.event).Add(1.0)
				} else {
					// Report Result
					switch resp.StatusCode {
					case 200:
						delivered200.With("event", work.event).Add(1.0)
					case 201:
						delivered201.With("event", work.event).Add(1.0)
					case 202:
						delivered202.With("event", work.event).Add(1.0)
					case 204:
						delivered204.With("event", work.event).Add(1.0)
					default:
						obs.getDeliveryCounter(resp.StatusCode).With("event", work.event).Add(1.0)
					}

					// read until the response is complete before closing to allow
					// connection reuse
					if nil != resp.Body {
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}
				}
			}
		} else {
			obs.droppedExpiredCounter.Add(1.0)
		}
	}
}

// queueOverflow handles the logic of what to do when a queue overflows
func (obs *CaduceusOutboundSender) queueOverflow() {
	obs.mutex.Lock()
	obs.dropUntil = time.Now().Add(obs.cutOffPeriod)
	obs.mutex.Unlock()

	var (
		debugLog = logging.Debug(obs.logger)
		errorLog = logging.Error(obs.logger)
	)

	obs.cutOffCounter.Add(1.0)
	debugLog.Log(logging.MessageKey(), "Queue overflowed", "url", obs.listener.Config.URL)

	msg, err := json.Marshal(obs.failureMsg)
	if nil != err {
		errorLog.Log(logging.MessageKey(), "Cut-off notification json.Marshall failed", "failureMessage", obs.failureMsg,
			"for", obs.listener.Config.URL, logging.ErrorKey(), err)
	} else {
		errorLog.Log(logging.MessageKey(), "Cut-off notification", "failureMessage", msg, "for", obs.listener.Config.URL)

		// Send a "you've been cut off" warning message
		if "" != obs.listener.FailureURL {

			payload := bytes.NewReader(msg)
			req, err := http.NewRequest("POST", obs.listener.FailureURL, payload)
			req.Header.Set("Content-Type", "application/json")

			if nil != obs.secret {
				h := hmac.New(sha1.New, obs.secret)
				h.Write(msg)
				sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
				req.Header.Set("X-Webpa-Signature", sig)
			}

			resp, err := obs.client.Do(req)
			if nil != err {
				// Failure
				errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
					obs.listener.FailureURL, "for", obs.listener.Config.URL, logging.ErrorKey(), err)
			} else {
				if nil == resp {
					// Failure
					errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification, nil response",
						"notification", obs.listener.FailureURL)
				} else {
					// Success
					logging.Info(obs.logger).Log("Able to send cut-off notification", "url", obs.listener.FailureURL,
						"status", resp.Status)
				}
			}
		} else {
			errorLog.Log(logging.MessageKey(), "No cut-off notification URL specified", "for", obs.listener.Config.URL)
		}
	}
}
