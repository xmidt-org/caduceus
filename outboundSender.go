/**
 * Copyright 2020 Comcast Cable Communications Management, LLC
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
	"container/ring"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/webpa-common/v2/device"

	"github.com/xmidt-org/webpa-common/v2/semaphore"
	"github.com/xmidt-org/webpa-common/v2/xhttp"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
)

// failureText is human readable text for the failure message
const failureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

// FailureMessage is a helper that lets us easily create a json struct to send
// when we have to cut and endpoint off.
type FailureMessage struct {
	Text         string                `json:"text"`
	Original     ancla.InternalWebhook `json:"webhook_registration"`
	CutOffPeriod string                `json:"cut_off_period"`
	QueueSize    int                   `json:"queue_size"`
	Workers      int                   `json:"worker_count"`
}

// OutboundSenderFactory is a configurable factory for OutboundSender objects.
type OutboundSenderFactory struct {
	// The WebHookListener to service
	Listener ancla.InternalWebhook

	// The http client Do() function to use for outbound requests.
	// Sender func(*http.Request) (*http.Response, error)
	Sender httpClient

	//
	ClientMiddleware func(httpClient) httpClient

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
	Logger *zap.Logger

	// CustomPIDs is a custom list of allowed PartnerIDs that will be used if a message
	// has no partner IDs.
	CustomPIDs []string

	// DisablePartnerIDs dictates whether or not to enforce the partner ID check.
	DisablePartnerIDs bool

	QueryLatency metrics.Histogram
}

type OutboundSender interface {
	Update(ancla.InternalWebhook) error
	Shutdown(bool)
	RetiredSince() time.Time
	Queue(*wrp.Message)
}

// CaduceusOutboundSender is the outbound sender object.
type CaduceusOutboundSender struct {
	id                               string
	urls                             *ring.Ring
	listener                         ancla.InternalWebhook
	deliverUntil                     time.Time
	dropUntil                        time.Time
	sender                           httpClient
	events                           []*regexp.Regexp
	matcher                          []*regexp.Regexp
	queueSize                        int
	deliveryRetries                  int
	deliveryInterval                 time.Duration
	deliveryCounter                  metrics.Counter
	deliveryRetryCounter             metrics.Counter
	droppedQueueFullCounter          metrics.Counter
	droppedCutoffCounter             metrics.Counter
	droppedExpiredCounter            metrics.Counter
	droppedExpiredBeforeQueueCounter metrics.Counter
	droppedNetworkErrCounter         metrics.Counter
	droppedInvalidConfig             metrics.Counter
	droppedPanic                     metrics.Counter
	cutOffCounter                    metrics.Counter
	queueDepthGauge                  metrics.Gauge
	renewalTimeGauge                 metrics.Gauge
	deliverUntilGauge                metrics.Gauge
	dropUntilGauge                   metrics.Gauge
	maxWorkersGauge                  metrics.Gauge
	currentWorkersGauge              metrics.Gauge
	deliveryRetryMaxGauge            metrics.Gauge
	wg                               sync.WaitGroup
	cutOffPeriod                     time.Duration
	workers                          semaphore.Interface
	maxWorkers                       int
	failureMsg                       FailureMessage
	logger                           *zap.Logger
	mutex                            sync.RWMutex
	queue                            atomic.Value
	customPIDs                       []string
	disablePartnerIDs                bool
	clientMiddleware                 func(httpClient) httpClient
}

// New creates a new OutboundSender object from the factory, or returns an error.
func (osf OutboundSenderFactory) New() (obs OutboundSender, err error) {
	if _, err = url.ParseRequestURI(osf.Listener.Webhook.Config.URL); nil != err {
		return
	}

	if nil == osf.ClientMiddleware {
		osf.ClientMiddleware = nopHTTPClient
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

	decoratedLogger := osf.Logger.With(zap.String("webhook.address", osf.Listener.Webhook.Address))

	caduceusOutboundSender := &CaduceusOutboundSender{
		id:               osf.Listener.Webhook.Config.URL,
		listener:         osf.Listener,
		sender:           osf.Sender,
		queueSize:        osf.QueueSize,
		cutOffPeriod:     osf.CutOffPeriod,
		deliverUntil:     osf.Listener.Webhook.Until,
		logger:           decoratedLogger,
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
		customPIDs:        osf.CustomPIDs,
		disablePartnerIDs: osf.DisablePartnerIDs,
		clientMiddleware:  osf.ClientMiddleware,
	}

	// Don't share the secret with others when there is an error.
	caduceusOutboundSender.failureMsg.Original.Webhook.Config.Secret = "XxxxxX"

	CreateOutbounderMetrics(osf.MetricsRegistry, caduceusOutboundSender)

	// update queue depth and current workers gauge to make sure they start at 0
	caduceusOutboundSender.queueDepthGauge.Set(0)
	caduceusOutboundSender.currentWorkersGauge.Set(0)

	caduceusOutboundSender.queue.Store(make(chan *wrp.Message, osf.QueueSize))

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
func (obs *CaduceusOutboundSender) Update(wh ancla.InternalWebhook) (err error) {

	// Validate the failure URL, if present
	if "" != wh.Webhook.FailureURL {
		if _, err = url.ParseRequestURI(wh.Webhook.FailureURL); nil != err {
			return
		}
	}

	// Create and validate the event regex objects
	// nolint:prealloc
	var events []*regexp.Regexp
	for _, event := range wh.Webhook.Events {
		var re *regexp.Regexp
		if re, err = regexp.Compile(event); nil != err {
			return
		}

		events = append(events, re)
	}
	if len(events) < 1 {
		err = errors.New("events must not be empty.")
		return
	}

	// Create the matcher regex objects
	matcher := []*regexp.Regexp{}
	for _, item := range wh.Webhook.Matcher.DeviceID {
		if ".*" == item {
			// Match everything - skip the filtering
			matcher = []*regexp.Regexp{}
			break
		}

		var re *regexp.Regexp
		if re, err = regexp.Compile(item); nil != err {
			err = fmt.Errorf("invalid matcher item: '%s'", item)
			return
		}
		matcher = append(matcher, re)
	}

	// Validate the various urls
	urlCount := len(wh.Webhook.Config.AlternativeURLs)
	for i := 0; i < urlCount; i++ {
		_, err = url.Parse(wh.Webhook.Config.AlternativeURLs[i])
		if err != nil {
			obs.logger.Error("failed to update url", zap.Any("url", wh.Webhook.Config.AlternativeURLs[i]), zap.Error(err))
			return
		}
	}

	obs.renewalTimeGauge.Set(float64(time.Now().Unix()))

	// write/update obs
	obs.mutex.Lock()

	obs.listener = wh

	obs.failureMsg.Original = wh
	// Don't share the secret with others when there is an error.
	obs.failureMsg.Original.Webhook.Config.Secret = "XxxxxX"

	obs.listener.Webhook.FailureURL = wh.Webhook.FailureURL
	obs.deliverUntil = wh.Webhook.Until
	obs.deliverUntilGauge.Set(float64(obs.deliverUntil.Unix()))

	obs.events = events

	obs.deliveryRetryMaxGauge.Set(float64(obs.deliveryRetries))

	// if matcher list is empty set it nil for Queue() logic
	obs.matcher = nil
	if 0 < len(matcher) {
		obs.matcher = matcher
	}

	if 0 == urlCount {
		obs.urls = ring.New(1)
		obs.urls.Value = obs.id
	} else {
		r := ring.New(urlCount)
		for i := 0; i < urlCount; i++ {
			r.Value = wh.Webhook.Config.AlternativeURLs[i]
			r = r.Next()
		}
		obs.urls = r
	}

	// Randomize where we start so all the instances don't synchronize
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	offset := r.Intn(obs.urls.Len())
	for 0 < offset {
		obs.urls = obs.urls.Next()
		offset--
	}

	// Update this here in case we make this configurable later
	obs.maxWorkersGauge.Set(float64(obs.maxWorkers))

	obs.mutex.Unlock()

	return
}

// Shutdown causes the CaduceusOutboundSender to stop its activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (obs *CaduceusOutboundSender) Shutdown(gentle bool) {
	if !gentle {
		// need to close the channel we're going to replace, in case it doesn't
		// have any events in it.
		close(obs.queue.Load().(chan *wrp.Message))
		obs.Empty(obs.droppedExpiredCounter)
	}
	close(obs.queue.Load().(chan *wrp.Message))
	obs.wg.Wait()

	obs.mutex.Lock()
	obs.deliverUntil = time.Time{}
	obs.deliverUntilGauge.Set(float64(obs.deliverUntil.Unix()))
	obs.queueDepthGauge.Set(0) //just in case
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

func overlaps(sl1 []string, sl2 []string) bool {
	for _, s1 := range sl1 {
		for _, s2 := range sl2 {
			if s1 == s2 {
				return true
			}
		}
	}
	return false
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

	if !obs.isValidTimeWindow(now, dropUntil, deliverUntil) {
		obs.logger.Debug("invalid time window for event", zap.Any("now", now), zap.Any("dropUntil", dropUntil), zap.Any("deliverUntil", deliverUntil))
		return
	}

	//check the partnerIDs
	if !obs.disablePartnerIDs {
		if len(msg.PartnerIDs) == 0 {
			msg.PartnerIDs = obs.customPIDs
		}
		if !overlaps(obs.listener.PartnerIDs, msg.PartnerIDs) {
			obs.logger.Debug("parter id check failed", zap.Strings("webhook.partnerIDs", obs.listener.PartnerIDs), zap.Strings("event.partnerIDs", msg.PartnerIDs))
			return
		}
	}

	var (
		matchEvent  bool
		matchDevice = true
	)
	for _, eventRegex := range events {
		if eventRegex.MatchString(strings.TrimPrefix(msg.Destination, "event:")) {
			matchEvent = true
			break
		}
	}
	if !matchEvent {
		obs.logger.Debug("destination regex doesn't match", zap.Strings("webhook.events", obs.listener.Webhook.Events), zap.String("event.dest", msg.Destination))
		return
	}

	if matcher != nil {
		matchDevice = false
		for _, deviceRegex := range matcher {
			if deviceRegex.MatchString(msg.Source) {
				matchDevice = true
				break
			}
		}
	}

	if !matchDevice {
		obs.logger.Debug("device regex doesn't match", zap.Strings("webhook.devices", obs.listener.Webhook.Matcher.DeviceID), zap.String("event.source", msg.Source))
		return
	}

	select {
	case obs.queue.Load().(chan *wrp.Message) <- msg:
		obs.queueDepthGauge.Add(1.0)
		obs.logger.Debug("event added to outbound queue", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
	default:
		obs.logger.Debug("queue full. event dropped", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
		obs.queueOverflow()
		obs.droppedQueueFullCounter.Add(1.0)
	}
}

func (obs *CaduceusOutboundSender) isValidTimeWindow(now, dropUntil, deliverUntil time.Time) bool {
	if !now.After(dropUntil) {
		// client was cut off
		obs.droppedCutoffCounter.Add(1.0)
		return false
	}

	if !now.Before(deliverUntil) {
		// outside delivery window
		obs.droppedExpiredBeforeQueueCounter.Add(1.0)
		return false
	}

	return true
}

// Empty is called on cutoff or shutdown and swaps out the current queue for
// a fresh one, counting any current messages in the queue as dropped.
// It should never close a queue, as a queue not referenced anywhere will be
// cleaned up by the garbage collector without needing to be closed.
func (obs *CaduceusOutboundSender) Empty(droppedCounter metrics.Counter) {
	droppedMsgs := obs.queue.Load().(chan *wrp.Message)
	obs.queue.Store(make(chan *wrp.Message, obs.queueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	obs.queueDepthGauge.Set(0.0)
}

func (obs *CaduceusOutboundSender) dispatcher() {
	defer obs.wg.Done()
	var (
		msg            *wrp.Message
		urls           *ring.Ring
		secret, accept string
		ok             bool
	)

Loop:
	for {
		// Always pull a new queue in case we have been cutoff or are shutting
		// down.
		msgQueue := obs.queue.Load().(chan *wrp.Message)
		// nolint:gosimple
		select {
		// The dispatcher cannot get stuck blocking here forever (caused by an
		// empty queue that is replaced and then Queue() starts adding to the
		// new queue) because:
		// 	- queue is only replaced on cutoff and shutdown
		//  - on cutoff, the first queue is always full so we will definitely
		//    get a message, drop it because we're cut off, then get the new
		//    queue and block until the cut off ends and Queue() starts queueing
		//    messages again.
		//  - on graceful shutdown, the queue is closed and then the dispatcher
		//    will send all messages, then break the loop, gather workers, and
		//    exit.
		//  - on non graceful shutdown, the queue is closed and then replaced
		//    with a new, empty queue that is also closed.
		//      - If the first queue is empty, we immediately break the loop,
		//        gather workers, and exit.
		//      - If the first queue has messages, we drop a message as expired
		//        pull in the new queue which is empty and closed, break the
		//        loop, gather workers, and exit.
		case msg, ok = <-msgQueue:
			// This is only true when a queue is empty and closed, which for us
			// only happens on Shutdown().
			if !ok {
				break Loop
			}
			obs.queueDepthGauge.Add(-1.0)
			obs.mutex.RLock()
			urls = obs.urls
			// Move to the next URL to try 1st the next time.
			// This is okay because we run a single dispatcher and it's the
			// only one updating this field.
			obs.urls = obs.urls.Next()
			deliverUntil := obs.deliverUntil
			dropUntil := obs.dropUntil
			secret = obs.listener.Webhook.Config.Secret
			accept = obs.listener.Webhook.Config.ContentType
			obs.mutex.RUnlock()

			now := time.Now()

			if now.Before(dropUntil) {
				obs.droppedCutoffCounter.Add(1.0)
				continue
			}
			if now.After(deliverUntil) {
				obs.Empty(obs.droppedExpiredCounter)
				continue
			}
			obs.workers.Acquire()
			obs.currentWorkersGauge.Add(1.0)

			go obs.send(urls, secret, accept, msg)
		}
	}
	for i := 0; i < obs.maxWorkers; i++ {
		obs.workers.Acquire()
	}
}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (obs *CaduceusOutboundSender) send(urls *ring.Ring, secret, acceptType string, msg *wrp.Message) {
	defer func() {
		if r := recover(); nil != r {
			obs.droppedPanic.Add(1.0)
			obs.logger.Error("goroutine send() panicked", zap.String("id", obs.id), zap.Any("panic", r))
		}
		obs.workers.Release()
		obs.currentWorkersGauge.Add(-1.0)
	}()

	payload := msg.Payload
	body := payload
	var payloadReader *bytes.Reader

	// Use the internal content type unless the accept type is wrp
	contentType := msg.ContentType
	switch acceptType {
	case "wrp", wrp.MimeTypeMsgpack, wrp.MimeTypeWrp:
		// WTS - We should pass the original, raw WRP event instead of
		// re-encoding it.
		contentType = wrp.MimeTypeMsgpack
		buffer := bytes.NewBuffer([]byte{})
		encoder := wrp.NewEncoder(buffer, wrp.Msgpack)
		encoder.Encode(msg)
		body = buffer.Bytes()
	}
	payloadReader = bytes.NewReader(body)

	req, err := http.NewRequest("POST", urls.Value.(string), payloadReader)
	if nil != err {
		// Report drop
		obs.droppedInvalidConfig.Add(1.0)
		obs.logger.Error("Invalid URL", zap.String("url", urls.Value.(string)), zap.String("id", obs.id), zap.Error(err))
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
		ShouldRetry:       xhttp.ShouldRetry,
		ShouldRetryStatus: xhttp.RetryCodes,
	}

	// update subsequent requests with the next url in the list upon failure
	retryOptions.UpdateRequest = func(request *http.Request) {
		urls = urls.Next()
		tmp, err := url.Parse(urls.Value.(string))
		if err != nil {
			obs.logger.Error("failed to update url", zap.String("url", urls.Value.(string)), zap.Error(err))
			return
		}
		request.URL = tmp
	}

	// Send it
	obs.logger.Debug("attempting to send event", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))

	retryer := xhttp.RetryTransactor(retryOptions, obs.sender.Do)
	client := obs.clientMiddleware(doerFunc(retryer))
	resp, err := client.Do(req)

	code := "failure"
	l := obs.logger
	if nil != err {
		// Report failure
		obs.droppedNetworkErrCounter.Add(1.0)
		l = obs.logger.With(zap.Error(err))
	} else {
		// Report Result
		code = strconv.Itoa(resp.StatusCode)

		// read until the response is complete before closing to allow
		// connection reuse
		if nil != resp.Body {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	obs.deliveryCounter.With("url", obs.id, "code", code, "event", event).Add(1.0)
	l.Debug("event sent-ish", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination), zap.String("code", code), zap.String("url", req.URL.String()))
}

// queueOverflow handles the logic of what to do when a queue overflows:
// cutting off the webhook for a time and sending a cut off notification
// to the failure URL.
func (obs *CaduceusOutboundSender) queueOverflow() {
	obs.mutex.Lock()
	if time.Now().Before(obs.dropUntil) {
		obs.mutex.Unlock()
		return
	}
	obs.dropUntil = time.Now().Add(obs.cutOffPeriod)
	obs.dropUntilGauge.Set(float64(obs.dropUntil.Unix()))
	secret := obs.listener.Webhook.Config.Secret
	failureMsg := obs.failureMsg
	failureURL := obs.listener.Webhook.FailureURL
	obs.mutex.Unlock()

	obs.cutOffCounter.Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	obs.Empty(obs.droppedCutoffCounter)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		obs.logger.Error("Cut-off notification json.Marshal failed", zap.Any("failureMessage", obs.failureMsg), zap.String("for", obs.id), zap.Error(err))
		return
	}

	// if no URL to send cut off notification to, do nothing
	if "" == failureURL {
		return
	}

	// Send a "you've been cut off" warning message
	payload := bytes.NewReader(msg)
	req, err := http.NewRequest("POST", failureURL, payload)
	if nil != err {
		// Failure
		obs.logger.Error("Unable to send cut-off notification", zap.String("notification",
			failureURL), zap.String("for", obs.id), zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", wrp.MimeTypeJson)

	if "" != secret {
		h := hmac.New(sha1.New, []byte(secret))
		h.Write(msg)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
		req.Header.Set("X-Webpa-Signature", sig)
	}

	resp, err := obs.sender.Do(req)
	if nil != err {
		// Failure
		obs.logger.Error("Unable to send cut-off notification", zap.String("notification", failureURL), zap.String("for", obs.id), zap.Error(err))
		return
	}

	if nil == resp {
		// Failure
		obs.logger.Error("Unable to send cut-off notification, nil response", zap.String("notification", failureURL))
		return
	}

	// Success

	if nil != resp.Body {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

	}
}
