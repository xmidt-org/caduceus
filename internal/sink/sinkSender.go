// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

import (
	"bytes"
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
	"github.com/xmidt-org/ancla"
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
	Text         string         `json:"text"`
	Original     ancla.Register `json:"webhook_registration"`
	CutOffPeriod string         `json:"cut_off_period"`
	QueueSize    int            `json:"queue_size"`
	Workers      int            `json:"worker_count"`
}

type Sender struct {
	id                string
	queueSize         int
	deliveryRetries   int
	maxWorkers        int
	disablePartnerIDs bool
	customPIDs        []string
	mutex             sync.RWMutex
	config            Config
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
	listener       ancla.Register
	matcher        Matcher
	metrics.Metrics
}

func NewSender(w *wrapper, l ancla.Register) (s *Sender, err error) {

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

	s = &Sender{
		id:           id,
		listener:     l,
		queueSize:    w.config.QueueSizePerSender,
		deliverUntil: l.GetUntil(),
		logger:       w.logger,
		config:       w.config, //TODO: need to figure out which config options are used for just Sender, just sink, and both
		// dropUntil:        where is this being set in old caduceus?,
		cutOffPeriod:     w.config.CutOffPeriod,
		deliveryRetries:  w.config.DeliveryRetries,
		deliveryInterval: w.config.DeliveryInterval,
		maxWorkers:       w.config.NumWorkersPerSender,
		Metrics:          w.metrics,
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

	s.OutgoingQueueDepth.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(0)
	s.ConsumerDeliveryWorkersGauge.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(0)

	// Don't share the secret with others when there is an error.
	hideSecret(s.failureMessage.Original)

	s.queue.Store(make(chan *wrp.Message, w.config.QueueSizePerSender))

	if err = s.Update(l); nil != err {
		return
	}

	s.workers = semaphore.New(s.maxWorkers)
	s.wg.Add(1)
	go s.dispatcher()

	return
}

func (s *Sender) Update(l ancla.Register) (err error) {
	s.matcher, err = NewMatcher(l, s.logger)
	sink, err := NewSink(s.config, s.logger, l)
	if err != nil {
		return err
	}

	s.sink = sink
	s.ConsumerRenewalTimeGauge.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(float64(time.Now().Unix()))

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.deliverUntil = l.GetUntil()
	s.ConsumerDeliverUntilGauge.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(float64(s.deliverUntil.Unix()))
	s.DeliveryRetryMaxGauge.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(float64(s.deliveryRetries))

	s.id = l.GetId()
	s.listener = l
	s.failureMessage.Original = l

	// Update this here in case we make this configurable later
	s.ConsumerMaxDeliveryWorkersGauge.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(float64(s.maxWorkers))

	return
}

// Queue is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
// TODO: can pass in message along with webhook information
func (s *Sender) Queue(msg *wrp.Message) {
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

		partnerIds, err := getPartnerIds(s.listener)
		if err == nil {
			if !overlaps(partnerIds, msg.PartnerIDs) {
				s.logger.Debug("partner id check failed", zap.Strings("webhook.partnerIDs", partnerIds), zap.Strings("event.partnerIDs", msg.PartnerIDs))
			}
		}

		if ok := s.matcher.IsMatch(msg); !ok {
			return
		}

	}

	//TODO: this code will be changing will take away queue and send to the sink interface (not webhook interface)
	select {
	case s.queue.Load().(chan *wrp.Message) <- msg:
		s.OutgoingQueueDepth.With(prometheus.Labels{metrics.UrlLabel: s.id}).Add(1.0)
		s.logger.Debug("event added to outbound queue", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
	default:
		s.logger.Debug("queue full. event dropped", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
		s.queueOverflow()
		s.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "queue_full"}).Add(1.0)
	}
}

// Shutdown causes the CaduceusOutboundSender to stop its activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (s *Sender) Shutdown(gentle bool) {
	if !gentle {
		// need to close the channel we're going to replace, in case it doesn't
		// have any events in it.
		close(s.queue.Load().(chan *wrp.Message))
		s.Empty(s.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "expired"}))
	}
	close(s.queue.Load().(chan *wrp.Message))
	s.wg.Wait()

	s.mutex.Lock()
	s.deliverUntil = time.Time{}
	s.ConsumerDeliverUntilGauge.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(float64(s.deliverUntil.Unix()))
	s.OutgoingQueueDepth.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(0) //just in case
	s.mutex.Unlock()
}

// RetiredSince returns the time the CaduceusOutboundSender retired (which could be in
// the future).
func (s *Sender) RetiredSince() (time.Time, error) {
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

func (s *Sender) isValidTimeWindow(now, dropUntil, deliverUntil time.Time) bool {
	if !now.After(dropUntil) {
		// client was cut off
		s.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "cut_off"}).Add(1.0)
		return false
	}

	if !now.Before(deliverUntil) {
		// outside delivery window
		s.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "expired_before_queueing"}).Add(1.0)
		return false
	}

	return true
}

// Empty is called on cutoff or shutdown and swaps out the current queue for
// a fresh one, counting any current messages in the queue as dropped.
// It should never close a queue, as a queue not referenced anywhere will be
// cleaned up by the garbage collector without needing to be closed.
func (s *Sender) Empty(droppedCounter prometheus.Counter) {
	droppedMsgs := s.queue.Load().(chan *wrp.Message)
	s.queue.Store(make(chan *wrp.Message, s.queueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	s.OutgoingQueueDepth.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(0.0)
}

// queueOverflow handles the logic of what to do when a queue overflows:
// cutting off the webhook for a time and sending a cut off notification
// to the failure URL.
func (s *Sender) queueOverflow() {
	s.mutex.Lock()
	if time.Now().Before(s.dropUntil) {
		s.mutex.Unlock()
		return
	}
	s.dropUntil = time.Now().Add(s.cutOffPeriod)
	s.ConsumerDropUntilGauge.With(prometheus.Labels{metrics.UrlLabel: s.id}).Set(float64(s.dropUntil.Unix()))
	//TODO: need to figure this out
	// secret := s.listener.Webhook.Config.Secret
	secret := "placeholderSecret"
	failureMsg := s.failureMessage
	//TODO: need to figure this out
	// failureURL := s.listener.Webhook.FailureURL
	failureURL := "placeholderURL"
	s.mutex.Unlock()

	s.CutOffCounter.With(prometheus.Labels{metrics.UrlLabel: s.id}).Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	s.Empty(s.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "cut_off"}))

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

func (s *Sender) dispatcher() {
	defer s.wg.Done()
	var (
		msg            *wrp.Message
		secret, accept string
		ok             bool
	)

	for {
		// Always pull a new queue in case we have been cutoff or are shutting
		// down.
		msgQueue := s.queue.Load().(chan *wrp.Message)
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
		msg, ok = <-msgQueue
		// This is only true when a queue is empty and closed, which for us
		// only happens on Shutdown().
		if !ok {
			break
		}
		s.OutgoingQueueDepth.With(prometheus.Labels{metrics.UrlLabel: s.id}).Add(-1.0)
		s.mutex.RLock()
		deliverUntil := s.deliverUntil
		dropUntil := s.dropUntil
		// secret = s.listener.Webhook.Config.Secret
		// accept = s.listener.Webhook.Config.ContentType
		s.mutex.RUnlock()

		now := time.Now()

		if now.Before(dropUntil) {
			s.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "cut_off"}).Add(1.0)
			continue
		}
		if now.After(deliverUntil) {
			s.Empty(s.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{metrics.UrlLabel: s.id, metrics.ReasonLabel: "expired"}))
			continue
		}
		s.workers.Acquire()
		s.ConsumerDeliveryWorkersGauge.With(prometheus.Labels{metrics.UrlLabel: s.id}).Add(1.0)

		go s.sink.Send(secret, accept, msg)
	}
	for i := 0; i < s.maxWorkers; i++ {
		s.workers.Acquire()
	}
}

func getPartnerIds(l ancla.Register) ([]string, error) {
	switch v := l.(type) {
	case *ancla.RegistryV1:
		return v.PartnerIDs, nil
	case *ancla.RegistryV2:
		return v.PartnerIds, nil
	default:
		return nil, fmt.Errorf("invalid register")
	}
}

func hideSecret(l ancla.Register) {
	switch v := l.(type) {
	case *ancla.RegistryV1:
		v.Registration.Config.Secret = "XxxxxX"
	case *ancla.RegistryV2:
		for i := range v.Registration.Webhooks {
			v.Registration.Webhooks[i].Secret = "XxxxxX"
		}
	}
}
