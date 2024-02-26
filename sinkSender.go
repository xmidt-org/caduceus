// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
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
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/xmidt-org/retry"
	"github.com/xmidt-org/retry/retryhttp"
	"github.com/xmidt-org/webpa-common/v2/semaphore"
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
	Text         string       `json:"text"`
	Original     ListenerStub `json:"webhook_registration"` //TODO: remove listener stub once ancla/argus issues fixed
	CutOffPeriod string       `json:"cut_off_period"`
	QueueSize    int          `json:"queue_size"`
	Workers      int          `json:"worker_count"`
}

type Sender interface {
	// Update(ancla.InternalWebhook) error
	Shutdown(bool)
	RetiredSince() time.Time
	Queue(*wrp.Message)
}

type SinkSenderMetrics struct {
	DeliveryCounter                 *prometheus.CounterVec
	DeliveryRetryCounter            *prometheus.CounterVec
	DeliveryRetryMaxGauge           *prometheus.GaugeVec
	CutOffCounter                   *prometheus.CounterVec
	SlowConsumerDroppedMsgCounter   *prometheus.CounterVec
	DropsDueToPanic                 *prometheus.CounterVec
	ConsumerDeliverUntilGauge       *prometheus.GaugeVec
	ConsumerDropUntilGauge          *prometheus.GaugeVec
	ConsumerDeliveryWorkersGauge    *prometheus.GaugeVec
	ConsumerMaxDeliveryWorkersGauge *prometheus.GaugeVec
	OutgoingQueueDepth              *prometheus.GaugeVec
	ConsumerRenewalTimeGauge        *prometheus.GaugeVec
	QueryLatency                    prometheus.ObserverVec
}

// CaduceusOutboundSender is the outbound sender object.
type SinkSender struct {
	id                               string
	urls                             *ring.Ring
	listener                         ListenerStub
	deliverUntil                     time.Time
	dropUntil                        time.Time
	client                           Client
	events                           []*regexp.Regexp
	matcher                          []*regexp.Regexp
	queueSize                        int
	deliveryRetries                  int
	deliveryInterval                 time.Duration
	deliveryCounter                  prometheus.CounterVec
	deliveryRetryCounter             *prometheus.CounterVec
	droppedQueueFullCounter          prometheus.Counter
	droppedCutoffCounter             prometheus.Counter
	droppedExpiredCounter            prometheus.Counter
	droppedExpiredBeforeQueueCounter prometheus.Counter
	droppedNetworkErrCounter         prometheus.Counter
	droppedInvalidConfig             prometheus.Counter
	droppedPanic                     prometheus.Counter
	cutOffCounter                    prometheus.Counter
	queueDepthGauge                  prometheus.Gauge
	renewalTimeGauge                 prometheus.Gauge
	deliverUntilGauge                prometheus.Gauge
	dropUntilGauge                   prometheus.Gauge
	maxWorkersGauge                  prometheus.Gauge
	currentWorkersGauge              prometheus.Gauge
	deliveryRetryMaxGauge            prometheus.Gauge
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
	clientMiddleware                 func(Client) Client
}

func newSinkSender(sw *SinkWrapper, listener ListenerStub, address string) (s *SinkSender, err error) {
	if sw.clientMiddleware == nil {
		sw.clientMiddleware = nopClient
	}
	if sw.client == nil {
		err = errors.New("nil Client")
		return
	}

	if sw.config.CutOffPeriod.Nanoseconds() == 0 {
		err = errors.New("invalid CutOffPeriod")
		return
	}

	if sw.logger == nil {
		err = errors.New("logger required")
		return
	}

	decoratedLogger := sw.logger.With(zap.String("webhook.address", address))

	sinkSender := &SinkSender{
		client:           sw.client,
		queueSize:        sw.config.QueueSizePerSender,
		cutOffPeriod:     sw.config.CutOffPeriod,
		logger:           decoratedLogger,
		deliveryRetries:  sw.config.DeliveryRetries,
		deliveryInterval: sw.config.DeliveryInterval,
		maxWorkers:       sw.config.NumWorkersPerSender,
		failureMsg: FailureMessage{
			Original:     listener,
			Text:         failureText,
			CutOffPeriod: sw.config.CutOffPeriod.String(),
			QueueSize:    sw.config.QueueSizePerSender,
			Workers:      sw.config.NumWorkersPerSender,
		},
		customPIDs:        sw.config.CustomPIDs,
		disablePartnerIDs: sw.config.DisablePartnerIDs,
		clientMiddleware:  sw.clientMiddleware,
	}

	//TODO: need to figure out how to set this up
	// Don't share the secret with others when there is an error.
	// sinkSender.failureMsg.Original.Webhook.Config.Secret = "XxxxxX"

	CreateOutbounderMetrics(sw.metrics, sinkSender)

	// update queue depth and current workers gauge to make sure they start at 0
	sinkSender.queueDepthGauge.Set(0)
	sinkSender.currentWorkersGauge.Set(0)

	sinkSender.queue.Store(make(chan *wrp.Message, sw.config.QueueSizePerSender))

	if err = listener.Registration.UpdateSender(sinkSender); nil != err {
		return
	} else {
		//TODO: is there a reason we have to call this here? we're already setting them above
		//Old caduceus does this
		sinkSender.listener = listener
		sinkSender.failureMsg.Original = listener
	}

	sinkSender.workers = semaphore.New(sinkSender.maxWorkers)
	sinkSender.wg.Add(1)
	go sinkSender.dispatcher()

	return sinkSender, err
}

// Shutdown causes the CaduceusOutboundSender to stop its activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (s *SinkSender) Shutdown(gentle bool) {
	if !gentle {
		// need to close the channel we're going to replace, in case it doesn't
		// have any events in it.
		close(s.queue.Load().(chan *wrp.Message))
		s.Empty(s.droppedExpiredCounter)
	}
	close(s.queue.Load().(chan *wrp.Message))
	s.wg.Wait()

	s.mutex.Lock()
	s.deliverUntil = time.Time{}
	s.deliverUntilGauge.Set(float64(s.deliverUntil.Unix()))
	s.queueDepthGauge.Set(0) //just in case
	s.mutex.Unlock()
}

// RetiredSince returns the time the CaduceusOutboundSender retired (which could be in
// the future).
func (s *SinkSender) RetiredSince() time.Time {
	s.mutex.RLock()
	deliverUntil := s.deliverUntil
	s.mutex.RUnlock()
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
func (s *SinkSender) Queue(msg *wrp.Message) {
	s.mutex.RLock()
	deliverUntil := s.deliverUntil
	dropUntil := s.dropUntil
	events := s.events
	matcher := s.matcher
	s.mutex.RUnlock()

	now := time.Now()

	if !s.isValidTimeWindow(now, dropUntil, deliverUntil) {
		s.logger.Debug("invalid time window for event", zap.Any("now", now), zap.Any("dropUntil", dropUntil), zap.Any("deliverUntil", deliverUntil))
		return
	}

	//check the partnerIDs
	if !s.disablePartnerIDs {
		if len(msg.PartnerIDs) == 0 {
			msg.PartnerIDs = s.customPIDs
		}
		// if !overlaps(s.listener.PartnerIDs, msg.PartnerIDs) {
		// 	s.logger.Debug("parter id check failed", zap.Strings("webhook.partnerIDs", s.listener.PartnerIDs), zap.Strings("event.partnerIDs", msg.PartnerIDs))
		// 	return
		// }
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
		s.logger.Debug("destination regex doesn't match", zap.String("event.dest", msg.Destination))
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
		s.logger.Debug("device regex doesn't match", zap.String("event.source", msg.Source))
		return
	}

	select {
	case s.queue.Load().(chan *wrp.Message) <- msg:
		s.queueDepthGauge.Add(1.0)
		s.logger.Debug("event added to outbound queue", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
	default:
		s.logger.Debug("queue full. event dropped", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
		s.queueOverflow()
		s.droppedQueueFullCounter.Add(1.0)
	}
}

func (s *SinkSender) isValidTimeWindow(now, dropUntil, deliverUntil time.Time) bool {
	if !now.After(dropUntil) {
		// client was cut off
		s.droppedCutoffCounter.Add(1.0)
		return false
	}

	if !now.Before(deliverUntil) {
		// outside delivery window
		s.droppedExpiredBeforeQueueCounter.Add(1.0)
		return false
	}

	return true
}

// Empty is called on cutoff or shutdown and swaps out the current queue for
// a fresh one, counting any current messages in the queue as dropped.
// It should never close a queue, as a queue not referenced anywhere will be
// cleaned up by the garbage collector without needing to be closed.
func (s *SinkSender) Empty(droppedCounter prometheus.Counter) {
	droppedMsgs := s.queue.Load().(chan *wrp.Message)
	s.queue.Store(make(chan *wrp.Message, s.queueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	s.queueDepthGauge.Set(0.0)
}

func (s *SinkSender) dispatcher() {
	defer s.wg.Done()
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
		msgQueue := s.queue.Load().(chan *wrp.Message)
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
			s.queueDepthGauge.Add(-1.0)
			s.mutex.RLock()
			urls = s.urls
			// Move to the next URL to try 1st the next time.
			// This is okay because we run a single dispatcher and it's the
			// only one updating this field.
			s.urls = s.urls.Next()
			deliverUntil := s.deliverUntil
			dropUntil := s.dropUntil
			// secret = s.listener.Webhook.Config.Secret
			// accept = s.listener.Webhook.Config.ContentType
			s.mutex.RUnlock()

			now := time.Now()

			if now.Before(dropUntil) {
				s.droppedCutoffCounter.Add(1.0)
				continue
			}
			if now.After(deliverUntil) {
				s.Empty(s.droppedExpiredCounter)
				continue
			}
			s.workers.Acquire()
			s.currentWorkersGauge.Add(1.0)

			go s.send(urls, secret, accept, msg)
		}
	}
	for i := 0; i < s.maxWorkers; i++ {
		s.workers.Acquire()
	}
}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (s *SinkSender) send(urls *ring.Ring, secret, acceptType string, msg *wrp.Message) {
	defer func() {
		if r := recover(); nil != r {
			s.droppedPanic.Add(1.0)
			s.logger.Error("goroutine send() panicked", zap.String("id", s.id), zap.Any("panic", r))
		}
		s.workers.Release()
		s.currentWorkersGauge.Add(-1.0)
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
		s.droppedInvalidConfig.Add(1.0)
		s.logger.Error("Invalid URL", zap.String("url", urls.Value.(string)), zap.String("id", s.id), zap.Error(err))
		return
	}

	req.Header.Set("Content-Type", contentType)

	// Add x-Midt-* headers
	wrphttp.AddMessageHeaders(req.Header, msg)

	// Provide the old headers for now
	req.Header.Set("X-Webpa-Event", strings.TrimPrefix(msg.Destination, "event:"))
	req.Header.Set("X-Webpa-Transaction-Id", msg.TransactionUUID)

	// Add the device id without the trailing service
	id, _ := wrp.ParseDeviceID(msg.Source)
	req.Header.Set("X-Webpa-Device-Id", string(id))
	req.Header.Set("X-Webpa-Device-Name", string(id))

	// Apply the secret

	if secret != "" {
		s := hmac.New(sha1.New, []byte(secret))
		s.Write(body)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(s.Sum(nil)))
		req.Header.Set("X-Webpa-Signature", sig)
	}

	// find the event "short name"
	event := msg.FindEventStringSubMatch()

	// Send it
	s.logger.Debug("attempting to send event", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
	client, _ := retryhttp.NewClient(
		retryhttp.WithHTTPClient(s.clientMiddleware(s.client)),
		retryhttp.WithRunner(s.addRunner(req, event)),
		retryhttp.WithRequesters(s.updateRequest(urls)),
	)
	resp, err := client.Do(req)

	code := "failure"
	l := s.logger
	if nil != err {
		// Report failure
		s.droppedNetworkErrCounter.Add(1.0)
		l = s.logger.With(zap.Error(err))
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
	s.deliveryCounter.With(prometheus.Labels{UrlLabel: s.id, CodeLabel: code, EventLabel: event}).Add(1.0)
	l.Debug("event sent-ish", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination), zap.String("code", code), zap.String("url", req.URL.String()))
}

// queueOverflow handles the logic of what to do when a queue overflows:
// cutting off the webhook for a time and sending a cut off notification
// to the failure URL.
func (s *SinkSender) queueOverflow() {
	s.mutex.Lock()
	if time.Now().Before(s.dropUntil) {
		s.mutex.Unlock()
		return
	}
	s.dropUntil = time.Now().Add(s.cutOffPeriod)
	s.dropUntilGauge.Set(float64(s.dropUntil.Unix()))
	// secret := s.listener.Webhook.Config.Secret
	secret := "placeholderSecret"
	failureMsg := s.failureMsg
	// failureURL := s.listener.Webhook.FailureURL
	failureURL := "placeholderURL"
	s.mutex.Unlock()

	s.cutOffCounter.Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	s.Empty(s.droppedCutoffCounter)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		s.logger.Error("Cut-off notification json.Marshal failed", zap.Any("failureMessage", s.failureMsg), zap.String("for", s.id), zap.Error(err))
		return
	}

	// if no URL to send cut off notification to, do nothing
	if failureURL == "" {
		return
	}

	// Send a "you've been cut off" warning message
	payload := bytes.NewReader(msg)
	req, err := http.NewRequest("POST", failureURL, payload)
	if nil != err {
		// Failure
		s.logger.Error("Unable to send cut-off notification", zap.String("notification",
			failureURL), zap.String("for", s.id), zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", wrp.MimeTypeJson)

	if secret != "" {
		h := hmac.New(sha1.New, []byte(secret))
		h.Write(msg)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
		req.Header.Set("X-Webpa-Signature", sig)
	}

	resp, err := s.client.Do(req)
	if nil != err {
		// Failure
		s.logger.Error("Unable to send cut-off notification", zap.String("notification", failureURL), zap.String("for", s.id), zap.Error(err))
		return
	}

	if nil == resp {
		// Failure
		s.logger.Error("Unable to send cut-off notification, nil response", zap.String("notification", failureURL))
		return
	}

	// Success

	if nil != resp.Body {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

	}
}

func (s *SinkSender) addRunner(request *http.Request, event string) retry.Runner[*http.Response] {
	//TODO: need to handle error
	runner, _ := retry.NewRunner[*http.Response](
		retry.WithPolicyFactory[*http.Response](retry.Config{
			Interval:   s.deliveryInterval,
			MaxRetries: s.deliveryRetries,
		}),
		retry.WithOnAttempt[*http.Response](s.onAttempt(request, event)),
	)
	return runner

}

func (s *SinkSender) updateRequest(urls *ring.Ring) func(*http.Request) *http.Request {
	return func(request *http.Request) *http.Request {
		urls = urls.Next()
		tmp, err := url.Parse(urls.Value.(string))
		if err != nil {
			s.logger.Error("failed to update url", zap.String(UrlLabel, urls.Value.(string)), zap.Error(err))
		}
		request.URL = tmp
		return request
	}
}

func (s *SinkSender) onAttempt(request *http.Request, event string) retry.OnAttempt[*http.Response] {

	return func(attempt retry.Attempt[*http.Response]) {
		if attempt.Retries > 0 {
			s.deliveryRetryCounter.With(prometheus.Labels{UrlLabel: s.id, EventLabel: event}).Add(1.0)
			s.logger.Debug("retrying HTTP transaction", zap.String("url", request.URL.String()), zap.Error(attempt.Err), zap.Int("retry", attempt.Retries+1), zap.Int("statusCode", attempt.Result.StatusCode))
		}

	}

}

func CreateOutbounderMetrics(m SinkSenderMetrics, c *SinkSender) {
	c.deliveryRetryCounter = m.DeliveryRetryCounter
	c.deliveryRetryMaxGauge = m.DeliveryRetryMaxGauge.With(prometheus.Labels{UrlLabel: c.id})
	c.cutOffCounter = m.CutOffCounter.With(prometheus.Labels{UrlLabel: c.id})
	c.droppedQueueFullCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: c.id, ReasonLabel: "queue_full"})
	c.droppedExpiredCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: c.id, ReasonLabel: "expired"})
	c.droppedExpiredBeforeQueueCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: c.id, ReasonLabel: "expired_before_queueing"})
	c.droppedCutoffCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: c.id, ReasonLabel: "cut_off"})
	c.droppedInvalidConfig = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: c.id, ReasonLabel: "invalid_config"})
	c.droppedNetworkErrCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: c.id, ReasonLabel: networkError})
	c.droppedPanic = m.DropsDueToPanic.With(prometheus.Labels{UrlLabel: c.id})
	c.queueDepthGauge = m.OutgoingQueueDepth.With(prometheus.Labels{UrlLabel: c.id})
	c.renewalTimeGauge = m.ConsumerRenewalTimeGauge.With(prometheus.Labels{UrlLabel: c.id})
	c.deliverUntilGauge = m.ConsumerDeliverUntilGauge.With(prometheus.Labels{UrlLabel: c.id})
	c.dropUntilGauge = m.ConsumerDropUntilGauge.With(prometheus.Labels{UrlLabel: c.id})
	c.currentWorkersGauge = m.ConsumerDeliveryWorkersGauge.With(prometheus.Labels{UrlLabel: c.id})
	c.maxWorkersGauge = m.ConsumerMaxDeliveryWorkersGauge.With(prometheus.Labels{UrlLabel: c.id})
}
