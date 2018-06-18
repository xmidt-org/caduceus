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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/webhook"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/Comcast/webpa-common/wrp/wrphttp"
	"github.com/Comcast/webpa-common/xhttp"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
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
	secret                   []byte
	secretChan               chan []byte
	events                   []*regexp.Regexp
	matcher                  []*regexp.Regexp
	queueSize                int
	queue                    chan *wrp.Message
	deliveryRetries          int
	deliveryInterval         time.Duration
	deliveryCounter          metrics.Counter
	deliveryRetryCounter     metrics.Counter
	droppedQueueFullCounter  metrics.Counter
	droppedExpiredCounter    metrics.Counter
	droppedNetworkErrCounter metrics.Counter
	droppedInvalidConfig     metrics.Counter
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
		secretChan:       make(chan []byte, 10),
		queueSize:        osf.QueueSize,
		cutOffPeriod:     osf.CutOffPeriod,
		deliverUntil:     osf.Listener.Until,
		logger:           osf.Logger,
		deliveryRetries:  osf.DeliveryRetries,
		deliveryInterval: osf.DeliveryInterval,
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

	caduceusOutboundSender.wg.Add(osf.NumWorkers)
	for i := 0; i < osf.NumWorkers; i++ {
		go caduceusOutboundSender.worker(i)
	}

	obs = caduceusOutboundSender
	return
}

// Update applies user configurable values for the outbound sender when a
// webhook is registered
func (obs *CaduceusOutboundSender) Update(wh webhook.W) (err error) {
	// make a copy
	obsCopy := *obs

	obsCopy.listener = wh
	obsCopy.failureMsg.Original = wh

	// Don't share the secret with others when there is an error.
	obsCopy.failureMsg.Original.Config.Secret = "XxxxxX"

	if "" != obsCopy.listener.Config.Secret {
		obsCopy.secret = []byte(obsCopy.listener.Config.Secret)
	}

	if "" != obsCopy.listener.FailureURL {
		if _, err = url.ParseRequestURI(obsCopy.listener.FailureURL); nil != err {
			return
		}
	}

	obsCopy.deliverUntil = obsCopy.listener.Until

	// Create the event regex objects
	for _, event := range obsCopy.listener.Events {
		var re *regexp.Regexp
		if re, err = regexp.Compile(event); nil != err {
			return
		}

		obsCopy.events = append(obsCopy.events, re)
	}
	if nil == obsCopy.events {
		err = errors.New("Events must not be empty.")
		return
	}

	// Create the matcher regex objects
	for _, item := range obsCopy.listener.Matcher.DeviceId {
		if ".*" == item {
			// Match everything - skip the filtering
			obsCopy.matcher = nil
			break
		}

		var re *regexp.Regexp
		if re, err = regexp.Compile(item); nil != err {
			err = fmt.Errorf("Invalid matcher item: '%s'", item)
			return
		}
		obsCopy.matcher = append(obsCopy.matcher, re)
	}

	// write/update obs
	obs.mutex.Lock()

	obs.listener = obsCopy.listener
	obs.failureMsg.Original = obsCopy.failureMsg.Original
	obs.failureMsg.Original.Config.Secret = obsCopy.failureMsg.Original.Config.Secret
	obs.secret = obsCopy.secret
	obs.listener.FailureURL = obsCopy.listener.FailureURL
	obs.deliverUntil = obsCopy.deliverUntil
	obs.events = obsCopy.events
	obs.matcher = obsCopy.matcher

	obs.mutex.Unlock()
	
	obs.secretChan <- obsCopy.secret

	return
}

// Shutdown causes the CaduceusOutboundSender to stop it's activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (obs *CaduceusOutboundSender) Shutdown(gentle bool) {
	close(obs.queue)
	close(obs.secretChan)
	
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

	if now.Before(deliverUntil) && now.After(dropUntil) {
		for _, eventRegex := range events {
			if eventRegex.MatchString(strings.TrimPrefix(msg.Destination, "event:")) {
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
					} else {
						obs.queueOverflow()
						obs.droppedQueueFullCounter.Add(1.0)
					}
				}
			} else {
				debugLog.Log(logging.MessageKey(),
					fmt.Sprintf("Regex did not match. got != expected: '%s' != '%s'\n",
						msg.Destination, eventRegex.String()))
			}
		}
	} else {
		debugLog.Log(logging.MessageKey(), "Outside delivery window")
		debugLog.Log("now", now, "before", deliverUntil, "after", dropUntil)
		obs.droppedExpiredCounter.Add(1.0)
	}
}

// helper function to get the right delivery counter to increment
func (obs *CaduceusOutboundSender) getCounter(c metrics.Counter, status int) metrics.Counter {
	if -1 == status {
		return c.With("url", obs.id, "code", "failure")
	}

	s := strconv.Itoa(status)
	return c.With("url", obs.id, "code", s)
}

type secretHash struct {
	value atomic.Value
}

func (sh *secretHash) set(h *hash.Hash) {
	sh.value.Store(h)
}

func (sh *secretHash) get() *hash.Hash {
	if h, ok := sh.value.Load().(*hash.Hash); ok {
		return h
	}

	return nil
}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (obs *CaduceusOutboundSender) worker(id int) {
	defer obs.wg.Done()

	// Make a local copy of the hmac
	var h secretHash
	h.set(new(hash.Hash))

	// routine that will listen for secret changes
	// if a change comes in, both the local secret copy and the hash are updated
	go func(sc chan []byte) {
		for {
			secret := <- sc
			// Create the base sha1 hash object for each thread
			if nil != secret {
				t := hmac.New(sha1.New, secret)
				h.set(&t)
			} else {
				h.set(new(hash.Hash))
			}
		}
	}(obs.secretChan)

	// Setup the retry structs once
	simpleCounter := &SimpleCounter{}

	retryOptions := xhttp.RetryOptions{
		Logger:   obs.logger,
		Retries:  obs.deliveryRetries,
		Interval: obs.deliveryInterval,
		Counter:  simpleCounter,
	}

	// Only optimize the successful answers
	delivered200 := obs.getCounter(obs.deliveryCounter, 200)
	delivered201 := obs.getCounter(obs.deliveryCounter, 201)
	delivered202 := obs.getCounter(obs.deliveryCounter, 202)
	delivered204 := obs.getCounter(obs.deliveryCounter, 204)
	retries200 := obs.getCounter(obs.deliveryRetryCounter, 200)
	retries201 := obs.getCounter(obs.deliveryRetryCounter, 201)
	retries202 := obs.getCounter(obs.deliveryRetryCounter, 202)
	retries204 := obs.getCounter(obs.deliveryRetryCounter, 204)

	for msg := range obs.queue {
		obs.queueDepthGauge.Add(-1.0)
		obs.mutex.RLock()
		deliverUntil := obs.deliverUntil
		dropUntil := obs.dropUntil
		obs.mutex.RUnlock()

		now := time.Now()
		if now.Before(deliverUntil) && now.After(dropUntil) {
			payload := msg.Payload
			payloadReader := bytes.NewReader(payload)
			req, err := http.NewRequest("POST", obs.id, payloadReader)
			if nil != err {
				// Report drop
				obs.droppedInvalidConfig.Add(1.0)
				logging.Error(obs.logger).Log(logging.MessageKey(), "Invalid URL", "url", obs.id,
					logging.ErrorKey(), err)
			} else {
				req.Header.Set("Content-Type", msg.ContentType)

				// Add x-Midt-* headers
				wrphttp.AddMessageHeaders(req.Header, msg)

				// Provide the old headers for now
				req.Header.Set("X-Webpa-Event", strings.TrimPrefix(msg.Destination, "event:"))
				req.Header.Set("X-Webpa-Transaction-Id", msg.TransactionUUID)

				// Add the device id without the trailing service
				id, _ := device.ParseID(msg.Source)
				req.Header.Set("X-Webpa-Device-Id", string(id))
				req.Header.Set("X-Webpa-Device-Name", string(id))

				// get the latest secret hash
				sh := *h.get()

				if nil != sh {
					sh.Reset()
					sh.Write(payload)
					sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(sh.Sum(nil)))
					req.Header.Set("X-Webpa-Signature", sig)
				}

				// Setup the retry logic
				simpleCounter.Count = 0.0

				// find the event "short name"
				match := eventPattern.FindStringSubmatch(msg.Destination)
				event := "unknown"
				if match != nil {
					event = match[1]
				}

				// Send it
				resp, err := xhttp.RetryTransactor(retryOptions, obs.sender)(req)
				if nil != err {
					// Report failure
					obs.getCounter(obs.deliveryCounter, -1).With("event", event).Add(1.0)
					obs.droppedNetworkErrCounter.Add(1.0)
				} else {
					// Report Result
					switch resp.StatusCode {
					case 200:
						delivered200.With("event", event).Add(1.0)
						retries200.With("event", event).Add(simpleCounter.Count)
					case 201:
						delivered201.With("event", event).Add(1.0)
						retries201.With("event", event).Add(simpleCounter.Count)
					case 202:
						delivered202.With("event", event).Add(1.0)
						retries202.With("event", event).Add(simpleCounter.Count)
					case 204:
						delivered204.With("event", event).Add(1.0)
						retries204.With("event", event).Add(simpleCounter.Count)
					default:
						obs.getCounter(obs.deliveryCounter, resp.StatusCode).With("event", event).Add(1.0)
						obs.getCounter(obs.deliveryRetryCounter, resp.StatusCode).With("event", event).Add(simpleCounter.Count)
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
	secret := obs.secret
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
	} else {
		errorLog.Log(logging.MessageKey(), "Cut-off notification", "failureMessage", msg, "for", obs.id)

		// Send a "you've been cut off" warning message
		if "" != failureURL {

			payload := bytes.NewReader(msg)
			req, err := http.NewRequest("POST", failureURL, payload)
			req.Header.Set("Content-Type", "application/json")

			if nil != obs.secret {
				h := hmac.New(sha1.New, secret)
				h.Write(msg)
				sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
				req.Header.Set("X-Webpa-Signature", sig)
			}

			resp, err := obs.sender(req)
			if nil != err {
				// Failure
				errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
					failureURL, "for", obs.id, logging.ErrorKey(), err)
			} else {
				if nil == resp {
					// Failure
					errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification, nil response",
						"notification", failureURL)
				} else {
					// Success
					logging.Info(obs.logger).Log("Able to send cut-off notification", "url", failureURL,
						"status", resp.Status)
				}
			}
		} else {
			errorLog.Log(logging.MessageKey(), "No cut-off notification URL specified", "for", obs.id)
		}
	}
}
