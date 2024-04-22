// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

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
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/caduceus/internal/client"
	"github.com/xmidt-org/caduceus/internal/metrics"
	"github.com/xmidt-org/webpa-common/v2/semaphore"
	"github.com/xmidt-org/wrp-go/v3"
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
	Text         string   `json:"text"`
	Original     Listener `json:"webhook_registration"` //TODO: remove listener stub once ancla/argus issues fixed
	CutOffPeriod string   `json:"cut_off_period"`
	QueueSize    int      `json:"queue_size"`
	Workers      int      `json:"worker_count"`
}

type Sender interface {
	Update(Listener) error
	Shutdown(bool)
	RetiredSince() (time.Time, error)
	Queue(*wrp.Message)
}
type sender struct {
	id                string
	queueSize         int
	deliveryRetries   int
	maxWorkers        int
	disablePartnerIDs bool
	customPIDs        []string
	mutex             sync.RWMutex
	deliverUntil      time.Time
	dropUntil         time.Time
	deliveryInterval  time.Duration
	cutOffPeriod      time.Duration
	queue             atomic.Value
	wg                sync.WaitGroup
	workers           semaphore.Interface
	logger            *zap.Logger
	sink              Sink
	// failureMessage is sent during a queue overflow.
	failureMessage FailureMessage
	listener       Listener
	matcher        Matcher
	SinkMetrics
}

type SinkMetrics struct {
	deliverUntilGauge     prometheus.Gauge
	deliveryRetryMaxGauge prometheus.Gauge
	renewalTimeGauge      prometheus.Gauge
	maxWorkersGauge       prometheus.Gauge
	// deliveryCounter                  prometheus.CounterVec
	deliveryRetryCounter             *prometheus.CounterVec
	droppedQueueFullCounter          prometheus.Counter
	droppedCutoffCounter             prometheus.Counter
	droppedExpiredCounter            prometheus.Counter
	droppedExpiredBeforeQueueCounter prometheus.Counter
	droppedMessage                   prometheus.Counter
	droppedInvalidConfig             prometheus.Counter
	droppedPanic                     prometheus.Counter
	cutOffCounter                    prometheus.Counter
	queueDepthGauge                  prometheus.Gauge
	dropUntilGauge                   prometheus.Gauge
	currentWorkersGauge              prometheus.Gauge
}

func NewSender(w *wrapper, l Listener) (s *sender, err error) {

	if w.clientMiddleware == nil {
		w.clientMiddleware = client.NopClient
	}
	if w.client == nil {
		err = errors.New("nil Client")
		return
	}

	if w.config.CutOffPeriod.Nanoseconds() == 0 {
		err = errors.New("invalid CutOffPeriod")
		return
	}

	if w.logger == nil {
		err = errors.New("logger required")
		return
	}
	id := l.GetId()

	s = &sender{
		id:           id,
		listener:     l,
		queueSize:    w.config.QueueSizePerSender,
		deliverUntil: l.GetUntil(),
		// dropUntil:        where is this being set in old caduceus?,
		cutOffPeriod:     w.config.CutOffPeriod,
		deliveryRetries:  w.config.DeliveryRetries,
		deliveryInterval: w.config.DeliveryInterval,
		maxWorkers:       w.config.NumWorkersPerSender,
		failureMessage: FailureMessage{
			Original:     l,
			Text:         failureText,
			CutOffPeriod: w.config.CutOffPeriod.String(),
			QueueSize:    w.config.QueueSizePerSender,
			Workers:      w.config.NumWorkersPerSender,
		},
		customPIDs:        w.config.CustomPIDs,
		disablePartnerIDs: w.config.DisablePartnerIDs,
	}

	s.CreateMetrics(w.metrics)
	s.queueDepthGauge.Set(0)
	s.currentWorkersGauge.Set(0)
	//TODO: need to figure out how to set this up
	// Don't share the secret with others when there is an error.
	// sinkSender.failureMsg.Original.Webhook.Config.Secret = "XxxxxX"

	s.queue.Store(make(chan *wrp.Message, w.config.QueueSizePerSender))

	if err = s.Update(l); nil != err {
		return
	}

	s.workers = semaphore.New(s.maxWorkers)
	s.wg.Add(1)
	go s.dispatcher()

	return
}

func (s *sender) Update(l Listener) (err error) {
	switch v := l.(type) {
	case *ListenerV1:
		m := &MatcherV1{}
		if err = m.Update(*v); err != nil {
			return
		}
		s.matcher = m
		NewWebhookV1(s)
	default:
		err = fmt.Errorf("invalid listner")
	}

	s.renewalTimeGauge.Set(float64(time.Now().Unix()))

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.deliverUntil = l.GetUntil()
	s.deliverUntilGauge.Set(float64(s.deliverUntil.Unix()))
	s.deliveryRetryMaxGauge.Set(float64(s.deliveryRetries))

	s.id = l.GetId()
	s.listener = l
	s.failureMessage.Original = l

	// Update this here in case we make this configurable later
	s.maxWorkersGauge.Set(float64(s.maxWorkers))

	return
}

// Queue is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
// TODO: can pass in message along with webhook information
func (s *sender) Queue(msg *wrp.Message) {
	s.mutex.RLock()
	deliverUntil := s.deliverUntil
	dropUntil := s.dropUntil
	s.mutex.RUnlock()

	now := time.Now()

	if !s.isValidTimeWindow(now, dropUntil, deliverUntil) {
		s.logger.Debug("invalid time window for event", zap.Any("now", now), zap.Any("dropUntil", dropUntil), zap.Any("deliverUntil", deliverUntil))
		return
	}

	if !s.disablePartnerIDs {
		if len(msg.PartnerIDs) == 0 {
			msg.PartnerIDs = s.customPIDs
		}
		if !overlaps(s.listener.GetPartnerIds(), msg.PartnerIDs) {
			s.logger.Debug("parter id check failed", zap.Strings("webhook.partnerIDs", s.listener.GetPartnerIds()), zap.Strings("event.partnerIDs", msg.PartnerIDs))
			return
		}
		if ok := s.matcher.IsMatch(msg); !ok {
			return
		}

	}

	//TODO: this code will be changing will take away queue and send to the sink interface (not webhook interface)
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

// Shutdown causes the CaduceusOutboundSender to stop its activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (s *sender) Shutdown(gentle bool) {
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
func (s *sender) RetiredSince() (time.Time, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	deliverUntil := s.deliverUntil
	if deliverUntil.IsZero() {
		return time.Time{}, errors.New("deliverUntil is zero")
	}

	return deliverUntil, nil
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

func (s *sender) isValidTimeWindow(now, dropUntil, deliverUntil time.Time) bool {
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
func (s *sender) Empty(droppedCounter prometheus.Counter) {
	droppedMsgs := s.queue.Load().(chan *wrp.Message)
	s.queue.Store(make(chan *wrp.Message, s.queueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	s.queueDepthGauge.Set(0.0)
}

// queueOverflow handles the logic of what to do when a queue overflows:
// cutting off the webhook for a time and sending a cut off notification
// to the failure URL.
func (s *sender) queueOverflow() {
	s.mutex.Lock()
	if time.Now().Before(s.dropUntil) {
		s.mutex.Unlock()
		return
	}
	s.dropUntil = time.Now().Add(s.cutOffPeriod)
	s.dropUntilGauge.Set(float64(s.dropUntil.Unix()))
	//TODO: need to figure this out
	// secret := s.listener.Webhook.Config.Secret
	secret := "placeholderSecret"
	failureMsg := s.failureMessage
	//TODO: need to figure this out
	// failureURL := s.listener.Webhook.FailureURL
	failureURL := "placeholderURL"
	s.mutex.Unlock()

	s.cutOffCounter.Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	s.Empty(s.droppedCutoffCounter)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		s.logger.Error("Cut-off notification json.Marshal failed", zap.Any("failureMessage", s.failureMessage), zap.String("for", s.id), zap.Error(err))
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
	client := ClientMock{}
	resp, err := client.Do(req)
	// resp, err := w.client.Do(req)
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

func (s *sender) dispatcher() {
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
			urls = s.matcher.getUrls()
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

			go s.sink.Send(urls, secret, accept, msg)
		}
	}
	for i := 0; i < s.maxWorkers; i++ {
		s.workers.Acquire()
	}
}

func (s *sender) CreateMetrics(m metrics.Metrics) {
	s.deliveryRetryCounter = m.DeliveryRetryCounter
	s.deliveryRetryMaxGauge = m.DeliveryRetryMaxGauge.With(prometheus.Labels{metrics.UrlLabel: s.id})
	s.cutOffCounter = m.CutOffCounter.With(prometheus.Labels{metrics.UrlLabel: s.id})
	s.droppedQueueFullCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "queue_full"})
	s.droppedExpiredCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "expired"})
	s.droppedExpiredBeforeQueueCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "expired_before_queueing"})
	s.droppedCutoffCounter = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "cut_off"})
	s.droppedInvalidConfig = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "invalid_config"})
	s.droppedMessage = m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: metrics.NetworkError})
	s.droppedPanic = m.DropsDueToPanic.With(prometheus.Labels{metrics.UrlLabel: s.id})
	s.queueDepthGauge = m.OutgoingQueueDepth.With(prometheus.Labels{metrics.UrlLabel: s.id})
	s.renewalTimeGauge = m.ConsumerRenewalTimeGauge.With(prometheus.Labels{metrics.UrlLabel: s.id})
	s.deliverUntilGauge = m.ConsumerDeliverUntilGauge.With(prometheus.Labels{metrics.UrlLabel: s.id})
	s.dropUntilGauge = m.ConsumerDropUntilGauge.With(prometheus.Labels{metrics.UrlLabel: s.id})
	s.currentWorkersGauge = m.ConsumerDeliveryWorkersGauge.With(prometheus.Labels{metrics.UrlLabel: s.id})
	s.maxWorkersGauge = m.ConsumerMaxDeliveryWorkersGauge.With(prometheus.Labels{metrics.UrlLabel: s.id})

}
