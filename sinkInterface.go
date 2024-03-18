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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/retry"
	"github.com/xmidt-org/retry/retryhttp"
	"github.com/xmidt-org/webpa-common/v2/semaphore"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
	"go.uber.org/zap"
)

// TODO: will be deleting - this is just a stub
type ClientMock struct {
}

func (c *ClientMock) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{}, nil
}

type SinkMiddleware interface {
	// Do(*http.Request) (*http.Response, error)
	Update(ListenerStub)
	SetFailureMessage(FailureMessage)
	SetMetrics(SinkMetrics)
	SetCommonSink(CommonSink)
	Shutdown(bool)
	Empty(prometheus.Counter)
	Queue(*wrp.Message)
	dispatcher()
}

// SinkMetrics are the metrics added to sink with sink specific information (i.e. id)
type SinkMetrics struct {
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
}

type CommonSink struct {
	id                string
	mutex             sync.RWMutex
	listener          ListenerStub
	deliverUntil      time.Time
	dropUntil         time.Time
	failureMsg        FailureMessage
	logger            *zap.Logger
	queue             atomic.Value
	wg                sync.WaitGroup
	deliveryInterval  time.Duration
	queueSize         int
	deliveryRetries   int
	maxWorkers        int
	cutOffPeriod      time.Duration
	disablePartnerIDs bool
	customPIDs        []string
	workers           semaphore.Interface
	SinkMetrics
}

type KafkaSink struct {
	CommonSink
}
type WebhookSink struct {
	CommonSink
}

type LegacySink struct {
	renewalTimeGauge prometheus.Gauge
	events           []*regexp.Regexp
	matcher          []*regexp.Regexp
	urls             *ring.Ring
	CommonSink
}

func (l *LegacySink) SetCommonSink(cs CommonSink) {
	l.id = cs.id
	l.maxWorkers = cs.maxWorkers
	l.deliveryRetries = cs.deliveryRetries
	l.logger = cs.logger
	l.queue = cs.queue
	l.queueSize = cs.queueSize
	l.cutOffPeriod = cs.cutOffPeriod
	l.workers = semaphore.New(cs.maxWorkers)
	l.deliveryInterval = cs.deliveryInterval
	l.wg.Add(1)
}

func (l *LegacySink) RetiredSince() time.Time {
	l.mutex.RLock()
	deliverUntil := l.deliverUntil
	l.mutex.RUnlock()
	return deliverUntil
}

func (l *LegacySink) Shutdown(gentle bool) {
	if !gentle {
		// need to close the channel we're going to replace, in case it doesn't
		// have any events in it.
		close(l.queue.Load().(chan *wrp.Message))
		l.Empty(l.droppedExpiredCounter)
	}
	close(l.queue.Load().(chan *wrp.Message))
	l.wg.Wait()

	l.mutex.Lock()
	l.deliverUntil = time.Time{}
	l.deliverUntilGauge.Set(float64(l.deliverUntil.Unix()))
	l.queueDepthGauge.Set(0) //just in case
	l.mutex.Unlock()
}

func (l *LegacySink) Empty(droppedCounter prometheus.Counter) {
	droppedMsgs := l.queue.Load().(chan *wrp.Message)
	l.queue.Store(make(chan *wrp.Message, l.queueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	l.queueDepthGauge.Set(0.0)
}

func (l *LegacySink) Queue(msg *wrp.Message) {
	l.mutex.RLock()
	deliverUntil := l.deliverUntil
	dropUntil := l.dropUntil
	events := l.events
	matcher := l.matcher
	l.mutex.RUnlock()

	now := time.Now()

	if !l.isValidTimeWindow(now, dropUntil, deliverUntil) {
		l.logger.Debug("invalid time window for event", zap.Any("now", now), zap.Any("dropUntil", dropUntil), zap.Any("deliverUntil", deliverUntil))
		return
	}

	//check the partnerIDs
	if !l.disablePartnerIDs {
		if len(msg.PartnerIDs) == 0 {
			msg.PartnerIDs = l.customPIDs
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
		l.logger.Debug("destination regex doesn't match", zap.String("event.dest", msg.Destination))
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
		l.logger.Debug("device regex doesn't match", zap.String("event.source", msg.Source))
		return
	}

	select {
	case l.queue.Load().(chan *wrp.Message) <- msg:
		l.queueDepthGauge.Add(1.0)
		l.logger.Debug("event added to outbound queue", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
	default:
		l.logger.Debug("queue full. event dropped", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
		l.queueOverflow()
		l.droppedQueueFullCounter.Add(1.0)
	}
}

func (l *LegacySink) isValidTimeWindow(now, dropUntil, deliverUntil time.Time) bool {
	if !now.After(dropUntil) {
		// client was cut off
		l.droppedCutoffCounter.Add(1.0)
		return false
	}

	if !now.Before(deliverUntil) {
		// outside delivery window
		l.droppedExpiredBeforeQueueCounter.Add(1.0)
		return false
	}

	return true
}
func (l *LegacySink) queueOverflow() {
	l.mutex.Lock()
	if time.Now().Before(l.dropUntil) {
		l.mutex.Unlock()
		return
	}
	l.dropUntil = time.Now().Add(l.cutOffPeriod)
	l.dropUntilGauge.Set(float64(l.dropUntil.Unix()))
	// secret := s.listener.Webhook.Config.Secret
	secret := "placeholderSecret"
	failureMsg := l.failureMsg
	// failureURL := s.listener.Webhook.FailureURL
	failureURL := "placeholderURL"
	l.mutex.Unlock()

	l.cutOffCounter.Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	l.Empty(l.droppedCutoffCounter)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		l.logger.Error("Cut-off notification json.Marshal failed", zap.Any("failureMessage", l.failureMsg), zap.String("for", l.id), zap.Error(err))
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
		l.logger.Error("Unable to send cut-off notification", zap.String("notification",
			failureURL), zap.String("for", l.id), zap.Error(err))
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
	// resp, err := l.client.Do(req)
	if nil != err {
		// Failure
		l.logger.Error("Unable to send cut-off notification", zap.String("notification", failureURL), zap.String("for", l.id), zap.Error(err))
		return
	}

	if nil == resp {
		// Failure
		l.logger.Error("Unable to send cut-off notification, nil response", zap.String("notification", failureURL))
		return
	}

	// Success

	if nil != resp.Body {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

	}
}
func (l *LegacySink) Update(wh ListenerStub) error {

	l.logger = l.logger.With(zap.String("webhook.address", wh.Registration.GetAddress()))

	r, ok := wh.Registration.(*RegistrationV1)
	if !ok {
		return fmt.Errorf("cannot convert registration into registrationV1")
	}

	if r.FailureURL != "" {
		_, err := url.ParseRequestURI(r.FailureURL)
		return err
	}

	var events []*regexp.Regexp
	for _, event := range r.Events {
		var re *regexp.Regexp
		re, err := regexp.Compile(event)
		if err != nil {
			return err
		}
		events = append(events, re)
	}

	if len(events) < 1 {
		return errors.New("events must not be empty")
	}

	var matcher []*regexp.Regexp
	for _, item := range r.Matcher.DeviceID {
		if item == ".*" {
			// Match everything - skip the filtering
			matcher = []*regexp.Regexp{}
			break
		}

		var re *regexp.Regexp
		re, err := regexp.Compile(item)
		if err != nil {
			return fmt.Errorf("invalid matcher item: '%s'", item)
		}
		matcher = append(matcher, re)
	}

	// Validate the various urls
	urlCount := len(r.Config.AlternativeURLs)
	for i := 0; i < urlCount; i++ {
		_, err := url.Parse(r.Config.AlternativeURLs[i])
		if err != nil {
			// s.logger.Error("failed to update url", zap.Any("url", v1.Config.AlternativeURLs[i]), zap.Error(err))
			return err
		}
	}

	l.renewalTimeGauge.Set(float64(time.Now().Unix()))

	// write/update sink sender
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.id = r.Config.ReceiverURL
	l.listener = wh

	l.failureMsg.Original = wh

	l.deliverUntil = r.Until
	l.deliverUntilGauge.Set(float64(l.deliverUntil.Unix()))

	l.events = events

	l.deliveryRetryMaxGauge.Set(float64(l.deliveryRetries))

	// if matcher list is empty set it nil for Queue() logic
	l.matcher = nil
	if 0 < len(matcher) {
		l.matcher = matcher
	}

	if 0 == urlCount {
		l.urls = ring.New(1)
		l.urls.Value = l.id
	} else {
		ring := ring.New(urlCount)
		for i := 0; i < urlCount; i++ {
			ring.Value = r.Config.AlternativeURLs[i]
			ring = ring.Next()
		}
		l.urls = ring
	}

	// Randomize where we start so all the instances don't synchronize
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	offset := rand.Intn(l.urls.Len())
	for 0 < offset {
		l.urls = l.urls.Next()
		offset--
	}

	// Update this here in case we make this configurable later
	l.maxWorkersGauge.Set(float64(l.maxWorkers))

	return nil

}

func CreateFailureMessage(sc SinkConfig, l ListenerStub, s SinkMiddleware) {
	fm := FailureMessage{
		Original:     l,
		Text:         failureText,
		CutOffPeriod: sc.CutOffPeriod.String(),
		QueueSize:    sc.QueueSizePerSender,
		Workers:      sc.NumWorkersPerSender,
	}

	s.SetFailureMessage(fm)
}

func (l *LegacySink) SetFailureMessage(fm FailureMessage) {
	l.failureMsg = fm
}

func (l *LegacySink) SetMetrics(sm SinkMetrics) {
	l.SinkMetrics = sm
}
func CreateSinkMetrics(m Metrics, id string, s SinkMiddleware) {
	sm := SinkMetrics{
		deliveryRetryCounter:             m.DeliveryRetryCounter,
		deliveryRetryMaxGauge:            m.DeliveryRetryMaxGauge.With(prometheus.Labels{UrlLabel: id}),
		cutOffCounter:                    m.CutOffCounter.With(prometheus.Labels{UrlLabel: id}),
		droppedQueueFullCounter:          m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "queue_full"}),
		droppedExpiredCounter:            m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "expired"}),
		droppedExpiredBeforeQueueCounter: m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "expired_before_queueing"}),
		droppedCutoffCounter:             m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "cut_off"}),
		droppedInvalidConfig:             m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "invalid_config"}),
		droppedNetworkErrCounter:         m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: networkError}),
		droppedPanic:                     m.DropsDueToPanic.With(prometheus.Labels{UrlLabel: id}),
		queueDepthGauge:                  m.OutgoingQueueDepth.With(prometheus.Labels{UrlLabel: id}),
		renewalTimeGauge:                 m.ConsumerRenewalTimeGauge.With(prometheus.Labels{UrlLabel: id}),
		deliverUntilGauge:                m.ConsumerDeliverUntilGauge.With(prometheus.Labels{UrlLabel: id}),
		dropUntilGauge:                   m.ConsumerDropUntilGauge.With(prometheus.Labels{UrlLabel: id}),
		currentWorkersGauge:              m.ConsumerDeliveryWorkersGauge.With(prometheus.Labels{UrlLabel: id}),
		maxWorkersGauge:                  m.ConsumerMaxDeliveryWorkersGauge.With(prometheus.Labels{UrlLabel: id}),
	}

	// update queue depth and current workers gauge to make sure they start at 0
	sm.queueDepthGauge.Set(0)
	sm.currentWorkersGauge.Set(0)

	s.SetMetrics(sm)
}

func (l *LegacySink) onAttempt(request *http.Request, event string) retry.OnAttempt[*http.Response] {

	return func(attempt retry.Attempt[*http.Response]) {
		if attempt.Retries > 0 {
			l.deliveryRetryCounter.With(prometheus.Labels{UrlLabel: l.id, EventLabel: event}).Add(1.0)
			l.logger.Debug("retrying HTTP transaction", zap.String("url", request.URL.String()), zap.Error(attempt.Err), zap.Int("retry", attempt.Retries+1), zap.Int("statusCode", attempt.Result.StatusCode))
		}

	}

}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (l *LegacySink) send(urls *ring.Ring, secret, acceptType string, msg *wrp.Message) {
	defer func() {
		if r := recover(); nil != r {
			l.droppedPanic.Add(1.0)
			l.logger.Error("goroutine send() panicked", zap.String("id", l.id), zap.Any("panic", r))
		}
		l.workers.Release()
		l.currentWorkersGauge.Add(-1.0)
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
		l.droppedInvalidConfig.Add(1.0)
		l.logger.Error("Invalid URL", zap.String("url", urls.Value.(string)), zap.String("id", l.id), zap.Error(err))
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
	l.logger.Debug("attempting to send event", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
	client, _ := retryhttp.NewClient(
		// retryhttp.WithHTTPClient(l.clientMiddleware(l.client)),
		retryhttp.WithRunner(l.addRunner(req, event)),
		retryhttp.WithRequesters(l.updateRequest(urls)),
	)
	resp, err := client.Do(req)

	code := "failure"
	log := l.logger
	if nil != err {
		// Report failure
		l.droppedNetworkErrCounter.Add(1.0)
		log = l.logger.With(zap.Error(err))
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
	l.deliveryCounter.With(prometheus.Labels{UrlLabel: l.id, CodeLabel: code, EventLabel: event}).Add(1.0)
	log.Debug("event sent-ish", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination), zap.String("code", code), zap.String("url", req.URL.String()))
}

func (l *LegacySink) dispatcher() {
	defer l.wg.Done()
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
		msgQueue := l.queue.Load().(chan *wrp.Message)
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
			l.queueDepthGauge.Add(-1.0)
			l.mutex.RLock()
			urls = l.urls
			// Move to the next URL to try 1st the next time.
			// This is okay because we run a single dispatcher and it's the
			// only one updating this field.
			l.urls = l.urls.Next()
			deliverUntil := l.deliverUntil
			dropUntil := l.dropUntil
			// secret = s.listener.Webhook.Config.Secret
			// accept = s.listener.Webhook.Config.ContentType
			l.mutex.RUnlock()

			now := time.Now()

			if now.Before(dropUntil) {
				l.droppedCutoffCounter.Add(1.0)
				continue
			}
			if now.After(deliverUntil) {
				l.Empty(l.droppedExpiredCounter)
				continue
			}
			l.workers.Acquire()
			l.currentWorkersGauge.Add(1.0)

			go l.send(urls, secret, accept, msg)
		}
	}
	for i := 0; i < l.maxWorkers; i++ {
		l.workers.Acquire()
	}
}

func (l *LegacySink) addRunner(request *http.Request, event string) retry.Runner[*http.Response] {
	runner, _ := retry.NewRunner[*http.Response](
		retry.WithPolicyFactory[*http.Response](retry.Config{
			Interval:   l.deliveryInterval,
			MaxRetries: l.deliveryRetries,
		}),
		retry.WithOnAttempt[*http.Response](l.onAttempt(request, event)),
	)
	return runner
}

func (l *LegacySink) updateRequest(urls *ring.Ring) func(*http.Request) *http.Request {
	return func(request *http.Request) *http.Request {
		urls = urls.Next()
		tmp, err := url.Parse(urls.Value.(string))
		if err != nil {
			l.logger.Error("failed to update url", zap.String(UrlLabel, urls.Value.(string)), zap.Error(err))
		}
		request.URL = tmp
		return request
	}
}
