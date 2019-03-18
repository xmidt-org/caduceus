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
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/semaphore"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/Comcast/webpa-common/xhttp"
	"github.com/Comcast/wrp-go/wrp"
	"github.com/Comcast/wrp-go/wrp/wrphttp"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
)

// failureText is human readable text for the failure message
const failureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

// outboundRequest stores the outgoing request and assorted data that has been
// collected so far (to save on processing the information again).
type outboundRequest struct {
	msg         *wrp.Message
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

	// The http client Do() function to use for outbound requests.
	Sender func(*http.Request) (*http.Response, error)

	// The number of delivery workers to create and use.
	NumWorkers int

	// The queue depth to buffer events before we declare overflow, shut
	// off the message delivery, and basically put the endpoint in "timeout."
	QueueSize int

	// The amount of time to cut off the consumer if they don't keep up.
	// Must be greater then 0 seconds
	CutOffPeriod time.Duration

	// Number of delivery retries before giving up
	DeliveryRetries int

	// Time in between delivery retries
	DeliveryInterval time.Duration

	// Metrics registry.
	MetricsRegistry CaduceusMetricsRegistry

	// The logger to use.
	Logger log.Logger
}

type OutboundSender interface {
	Update(webhook.W) error
	Shutdown(bool)
	RetiredSince() time.Time
	Queue(*wrp.Message)
}

// CaduceusOutboundSender is the outbound sender object.
type CaduceusOutboundSender struct {
	id                       string
	listener                 webhook.W
	deliverUntil             time.Time
	dropUntil                time.Time
	sender                   func(*http.Request) (*http.Response, error)
	events                   []*regexp.Regexp
	matcher                  []*regexp.Regexp
	queueSize                int
	queue                    chan *wrp.Message
	deliveryRetries          int
	deliveryInterval         time.Duration
	deliveryCounter          metrics.Counter
	deliveryRetryCounter     metrics.Counter
	droppedQueueFullCounter  metrics.Counter
	droppedCutoffCounter     metrics.Counter
	droppedExpiredCounter    metrics.Counter
	droppedNetworkErrCounter metrics.Counter
	droppedInvalidConfig     metrics.Counter
	cutOffCounter            metrics.Counter
	queueDepthGauge          metrics.Gauge
	eventType                metrics.Counter
	wg                       sync.WaitGroup
	cutOffPeriod             time.Duration
	workers                  semaphore.Interface
	maxWorkers               int
	failureMsg               FailureMessage
	logger                   log.Logger
	mutex                    sync.RWMutex
}

// New creates a new OutboundSender object from the factory, or returns an error.
func (osf OutboundSenderFactory) New() (obs OutboundSender, err error) {
	if _, err = url.ParseRequestURI(osf.Listener.Config.URL); nil != err {
		return
	}

	if nil == osf.Sender {
		err = errors.New("nil Sender()")
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
		id:               osf.Listener.Config.URL,
		listener:         osf.Listener,
		sender:           osf.Sender,
		queueSize:        osf.QueueSize,
		cutOffPeriod:     osf.CutOffPeriod,
		deliverUntil:     osf.Listener.Until,
		logger:           osf.Logger,
		deliveryRetries:  osf.DeliveryRetries,
		deliveryInterval: osf.DeliveryInterval,
		maxWorkers:       osf.NumWorkers,
		failureMsg: FailureMessage{
			Original:     osf.Listener,
			Text:         failureText,
			CutOffPeriod: osf.CutOffPeriod.String(),
			QueueSize:    osf.QueueSize,
			Workers:      osf.NumWorkers,
		},
	}

	// Don't share the secret with others when there is an error.
	caduceusOutboundSender.failureMsg.Original.Config.Secret = "XxxxxX"

	caduceusOutboundSender.deliveryCounter = osf.MetricsRegistry.NewCounter(DeliveryCounter)
	caduceusOutboundSender.deliveryRetryCounter = osf.MetricsRegistry.NewCounter(DeliveryRetryCounter)

	caduceusOutboundSender.cutOffCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerCounter).With("url", caduceusOutboundSender.id)

	caduceusOutboundSender.droppedQueueFullCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerDroppedMsgCounter).With("url", caduceusOutboundSender.id, "reason", "queue_full")

	caduceusOutboundSender.droppedExpiredCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerDroppedMsgCounter).With("url", caduceusOutboundSender.id, "reason", "expired")

	caduceusOutboundSender.droppedCutoffCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerDroppedMsgCounter).With("url", caduceusOutboundSender.id, "reason", "cut_off")

	caduceusOutboundSender.droppedInvalidConfig = osf.MetricsRegistry.
		NewCounter(SlowConsumerDroppedMsgCounter).With("url", caduceusOutboundSender.id, "reason", "invalid_config")

	caduceusOutboundSender.droppedNetworkErrCounter = osf.MetricsRegistry.
		NewCounter(SlowConsumerDroppedMsgCounter).With("url", caduceusOutboundSender.id, "reason", "network_err")

	caduceusOutboundSender.queueDepthGauge = osf.MetricsRegistry.
		NewGauge(OutgoingQueueDepth).With("url", caduceusOutboundSender.id)

	// Give us some head room so that we don't block when we get near the
	// completely full point.
	caduceusOutboundSender.queue = make(chan *wrp.Message, osf.QueueSize)

	if err = caduceusOutboundSender.Update(osf.Listener); nil != err {
		return
	}

	caduceusOutboundSender.workers = semaphore.New(caduceusOutboundSender.maxWorkers)
	caduceusOutboundSender.wg.Add(1)
	go caduceusOutboundSender.dispatcher()

	obs = caduceusOutboundSender

	return
}

// Update applies user configurable values for the outbound sender when a
// webhook is registered
func (obs *CaduceusOutboundSender) Update(wh webhook.W) (err error) {

	// Validate the failure URL, if present
	if "" != wh.FailureURL {
		if _, err = url.ParseRequestURI(wh.FailureURL); nil != err {
			return
		}
	}

	// Create and validate the event regex objects
	var events []*regexp.Regexp
	for _, event := range wh.Events {
		var re *regexp.Regexp
		if re, err = regexp.Compile(event); nil != err {
			return
		}

		events = append(events, re)
	}
	if len(events) < 1 {
		err = errors.New("Events must not be empty.")
		return
	}

	// Create the matcher regex objects
	matcher := []*regexp.Regexp{}
	for _, item := range wh.Matcher.DeviceId {
		if ".*" == item {
			// Match everything - skip the filtering
			matcher = []*regexp.Regexp{}
			break
		}

		var re *regexp.Regexp
		if re, err = regexp.Compile(item); nil != err {
			err = fmt.Errorf("Invalid matcher item: '%s'", item)
			return
		}
		matcher = append(matcher, re)
	}

	// write/update obs
	obs.mutex.Lock()

	obs.listener = wh

	obs.failureMsg.Original = wh
	// Don't share the secret with others when there is an error.
	obs.failureMsg.Original.Config.Secret = "XxxxxX"

	obs.listener.FailureURL = wh.FailureURL
	obs.deliverUntil = wh.Until
	obs.events = events

	// if matcher list is empty set it nil for Queue() logic
	obs.matcher = nil
	if 0 < len(matcher) {
		obs.matcher = matcher
	}

	obs.mutex.Unlock()

	return
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

// Queue is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
func (obs *CaduceusOutboundSender) Queue(msg *wrp.Message) {
	obs.mutex.RLock()
	deliverUntil := obs.deliverUntil
	dropUntil := obs.dropUntil
	events := obs.events
	matcher := obs.matcher
	obs.mutex.RUnlock()

	now := time.Now()

	var debugLog = logging.Debug(obs.logger)
	if false == obs.isValidTimeWindow(now, dropUntil, deliverUntil) {
		return
	}

	for _, eventRegex := range events {
		if false == eventRegex.MatchString(strings.TrimPrefix(msg.Destination, "event:")) {
			debugLog.Log(logging.MessageKey(),
				fmt.Sprintf("Regex did not match. got != expected: '%s' != '%s'\n",
					msg.Destination, eventRegex.String()))

			continue
		}

		matchDevice := (nil == matcher)
		if nil != matcher {
			for _, deviceRegex := range matcher {
				if deviceRegex.MatchString(msg.Source) {
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
					if matchers, ok := matcher[key]; ok {
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
				obs.queueDepthGauge.Add(1.0)
				obs.queue <- msg
				debugLog.Log(logging.MessageKey(), "WRP Sent to obs queue", "url", obs.id)
				// a regex was matched, no need to check further matches
				break
			}
			obs.queueOverflow()
			obs.droppedQueueFullCounter.Add(1.0)
		}
	}
}

func (obs *CaduceusOutboundSender) isValidTimeWindow(now, dropUntil, deliverUntil time.Time) bool {
	var debugLog = logging.Debug(obs.logger)

	if false == now.After(dropUntil) {
		debugLog.Log(logging.MessageKey(), "Client has been cut off",
			"now", now, "before", deliverUntil, "after", dropUntil)
		obs.droppedCutoffCounter.Add(1.0)
		return false
	}

	if false == now.Before(deliverUntil) {
		debugLog.Log(logging.MessageKey(), "Outside delivery window",
			"now", now, "before", deliverUntil, "after", dropUntil)
		obs.droppedExpiredCounter.Add(1.0)
		return false
	}

	return true
}

func (obs *CaduceusOutboundSender) dispatcher() {
	defer obs.wg.Done()

	for msg := range obs.queue {
		obs.queueDepthGauge.Add(-1.0)

		obs.mutex.RLock()
		deliverUntil := obs.deliverUntil
		dropUntil := obs.dropUntil
		secret := obs.listener.Config.Secret
		accept := obs.listener.Config.ContentType
		obs.mutex.RUnlock()

		now := time.Now()

		if now.Before(dropUntil) {
			obs.droppedCutoffCounter.Add(1.0)
			continue
		}
		if now.After(deliverUntil) {

			obs.droppedExpiredCounter.Add(1.0)
			continue
		}
		obs.workers.Acquire()
		go obs.send(secret, accept, msg)
	}

	// Grab all the workers to make sure they are done.
	for i := 0; i < obs.maxWorkers; i++ {
		obs.workers.Acquire()
	}
}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (obs *CaduceusOutboundSender) send(secret, acceptType string, msg *wrp.Message) {
	defer obs.workers.Release()

	payload := msg.Payload
	body := payload
	var payloadReader *bytes.Reader

	// Use the internal content type unless the accept type is wrp
	contentType := msg.ContentType
	switch acceptType {
	case "wrp", "application/msgpack", "application/wrp":
		// WTS - We should pass the original, raw WRP event instead of
		// re-encoding it.
		contentType = "application/msgpack"
		buffer := bytes.NewBuffer([]byte{})
		encoder := wrp.NewEncoder(buffer, wrp.Msgpack)
		encoder.Encode(msg)
		body = buffer.Bytes()
	}
	payloadReader = bytes.NewReader(body)

	req, err := http.NewRequest("POST", obs.id, payloadReader)
	if nil != err {
		// Report drop
		obs.droppedInvalidConfig.Add(1.0)
		logging.Error(obs.logger).Log(logging.MessageKey(), "Invalid URL", "url", obs.id,
			logging.ErrorKey(), err)
		return
	}

	req.Header.Set("Content-Type", contentType)

	// Add x-Midt-* headers
	wrphttp.AddMessageHeaders(req.Header, msg)

	// Provide the old headers for now
	req.Header.Set("X-Webpa-Event", strings.TrimPrefix(msg.Destination, "event:"))
	req.Header.Set("X-Webpa-Transaction-Id", msg.TransactionUUID)

	// Add the device id without the trailing service
	id, _ := device.ParseID(msg.Source)
	req.Header.Set("X-Webpa-Device-Id", string(id))
	req.Header.Set("X-Webpa-Device-Name", string(id))

	// Apply the secret

	if "" != secret {
		s := hmac.New(sha1.New, []byte(secret))
		s.Write(body)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(s.Sum(nil)))
		req.Header.Set("X-Webpa-Signature", sig)
	}

	// find the event "short name"
	event := msg.FindEventStringSubMatch()

	retryOptions := xhttp.RetryOptions{
		Logger:   obs.logger,
		Retries:  obs.deliveryRetries,
		Interval: obs.deliveryInterval,
		Counter:  obs.deliveryRetryCounter.With("url", obs.id, "event", event),
		// Always retry on failures up to the max count.
		ShouldRetry: func(error) bool { return true },
	}

	// Send it
	resp, err := xhttp.RetryTransactor(retryOptions, obs.sender)(req)
	code := "failure"
	if nil != err {
		// Report failure
		obs.droppedNetworkErrCounter.Add(1.0)
	} else {
		// Report Result
		code = strconv.Itoa(resp.StatusCode)

		// read until the response is complete before closing to allow
		// connection reuse
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	obs.deliveryCounter.With("url", obs.id, "code", code, "event", event).Add(1.0)
}

// queueOverflow handles the logic of what to do when a queue overflows
func (obs *CaduceusOutboundSender) queueOverflow() {
	obs.mutex.Lock()
	obs.dropUntil = time.Now().Add(obs.cutOffPeriod)
	secret := obs.listener.Config.Secret
	failureMsg := obs.failureMsg
	failureURL := obs.listener.FailureURL
	obs.mutex.Unlock()

	var (
		debugLog = logging.Debug(obs.logger)
		errorLog = logging.Error(obs.logger)
	)

	obs.cutOffCounter.Add(1.0)
	debugLog.Log(logging.MessageKey(), "Queue overflowed", "url", obs.id)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		errorLog.Log(logging.MessageKey(), "Cut-off notification json.Marshall failed", "failureMessage", obs.failureMsg,
			"for", obs.id, logging.ErrorKey(), err)
		return
	}
	errorLog.Log(logging.MessageKey(), "Cut-off notification", "failureMessage", msg, "for", obs.id)

	// Send a "you've been cut off" warning message
	if "" == failureURL {
		errorLog.Log(logging.MessageKey(), "No cut-off notification URL specified", "for", obs.id)
		return
	}

	payload := bytes.NewReader(msg)
	req, err := http.NewRequest("POST", failureURL, payload)
	if nil != err {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
			failureURL, "for", obs.id, logging.ErrorKey(), err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	if "" != secret {
		h := hmac.New(sha1.New, []byte(secret))
		h.Write(msg)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
		req.Header.Set("X-Webpa-Signature", sig)
	}

	resp, err := obs.sender(req)
	if nil != err {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
			failureURL, "for", obs.id, logging.ErrorKey(), err)
		return
	}

	if nil == resp {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification, nil response",
			"notification", failureURL)
		return
	}

	// Success
	logging.Info(obs.logger).Log("Able to send cut-off notification", "url", failureURL,
		"status", resp.Status)
}
