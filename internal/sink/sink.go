// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

import (
	"bytes"
	"container/ring"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/caduceus/internal/metrics"
	"github.com/xmidt-org/retry"
	"github.com/xmidt-org/retry/retryhttp"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
	"go.uber.org/zap"
)

type Sink interface {
	Send(string, string, *wrp.Message) error
}

type WebhookV1 struct {
	urls *ring.Ring
	CommonWebhook
	//TODO: need to determine best way to add client and client middleware to WebhooV1
	// client           http.Client
	// clientMiddleware func(http.Client) http.Client
}

type Webhooks []*WebhookV1
type CommonWebhook struct {
	id               string
	failureUrl       string
	deliveryInterval time.Duration
	deliveryRetries  int
	mutex            sync.RWMutex
	logger           *zap.Logger
}
type Kafkas []*Kafka
type Kafka struct {
	brokerAddr []string
	topic      string
	config     *sarama.Config
	producer   sarama.SyncProducer
	CommonWebhook
}

func NewSink(c Config, logger *zap.Logger, listener ancla.Register) Sink {
	switch l := listener.(type) {
	case *ancla.RegistryV1:
		v1 := &WebhookV1{}
		v1.Update(c, logger, l.Registration.Config.AlternativeURLs, l.GetId(), l.Registration.FailureURL, l.Registration.Config.ReceiverURL)
		return v1
	case *ancla.RegistryV2:
		if len(l.Registration.Webhooks) > 0 {
			var whs Webhooks
			for _, wh := range l.Registration.Webhooks {
				v1 := &WebhookV1{}
				v1.Update(c, logger, wh.ReceiverURLs[1:], l.GetId(), l.Registration.FailureURL, wh.ReceiverURLs[0])
				whs = append(whs, v1)
			}
			return whs
		}
		if len(l.Registration.Kafkas) > 0 {
			var sink Kafkas
			for _, k := range l.Registration.Kafkas {
				kafka := &Kafka{}
				kafka.Update(l.GetId(), "quickstart-events", k.RetryHint.MaxRetry, k.BootstrapServers, logger)
				sink = append(sink, kafka)
			}
			return sink
		}
	default:
		return nil
	}
	return nil
}

func (v1 *WebhookV1) Update(c Config, l *zap.Logger, altUrls []string, id, failureUrl, receiverUrl string) (err error) {
	v1.id = id
	v1.failureUrl = failureUrl
	v1.deliveryInterval = c.DeliveryInterval
	v1.deliveryRetries = c.DeliveryRetries
	v1.logger = l

	urlCount, err := getUrls(altUrls)
	if err != nil {
		l.Error("error recevied parsing urls", zap.Error(err))
	}
	v1.updateUrls(urlCount, receiverUrl, altUrls)

	return nil
}

func (v1 *WebhookV1) updateUrls(urlCount int, url string, urls []string) {
	v1.mutex.Lock()
	defer v1.mutex.Unlock()

	if urlCount == 0 {
		v1.urls = ring.New(1)
		v1.urls.Value = url
	} else {
		ring := ring.New(urlCount)
		for i := 0; i < urlCount; i++ {
			ring.Value = urls[i]
			ring = ring.Next()
		}
		v1.urls = ring
	}

	// Randomize where we start so all the instances don't synchronize
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	offset := rand.Intn(v1.urls.Len())
	for 0 < offset {
		v1.urls = v1.urls.Next()
		offset--
	}
}

func getUrls(urls []string) (int, error) {
	var errs error
	// Validate the various urls
	urlCount := 0
	for _, u := range urls {
		_, err := url.Parse(u)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to update url: %v; err: %v", u, err))
			continue
		}
		urlCount++
	}

	return urlCount, errs

}

func (whs Webhooks) Send(secret, acceptType string, msg *wrp.Message) error {
	var errs error

	for _, wh := range whs {
		err := wh.Send(secret, acceptType, msg)
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (v1 *WebhookV1) Send(secret, acceptType string, msg *wrp.Message) error {
	defer func() {
		if r := recover(); nil != r {
			// s.droppedPanic.Add(1.0)
			v1.logger.Error("goroutine send() panicked", zap.String("id", v1.id), zap.Any("panic", r))
		}
		// s.workers.Release()
		// s.currentWorkersGauge.Add(-1.0)
	}()

	//TODO: is there a reason we are setting it up like this?
	//this was how it was done in old caduceus
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

	req, err := http.NewRequest("POST", v1.urls.Value.(string), payloadReader)
	if err != nil {
		// Report drop
		// s.droppedInvalidConfig.Add(1.0)
		v1.logger.Error("Invalid URL", zap.String(metrics.UrlLabel, v1.urls.Value.(string)), zap.String("id", v1.id), zap.Error(err))
		return err
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
	v1.logger.Debug("attempting to send event", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
	client, _ := retryhttp.NewClient(
		// retryhttp.WithHTTPClient(s.clientMiddleware(s.client)),
		retryhttp.WithRunner(v1.addRunner(req, event)),
		retryhttp.WithRequesters(v1.updateRequest(v1.urls)),
	)
	resp, err := client.Do(req)

	var deliveryCounterLabels []string
	code := metrics.MessageDroppedCode
	reason := metrics.NoErrReason
	logger := v1.logger
	if err != nil {
		// Report failure
		//TODO: add droppedMessage to webhook metrics and remove from sink sender?
		// v1.droppedMessage.Add(1.0)
		reason = metrics.GetDoErrReason(err)
		if resp != nil {
			code = strconv.Itoa(resp.StatusCode)
		}

		logger = v1.logger.With(zap.String(metrics.ReasonLabel, reason), zap.Error(err))
		deliveryCounterLabels = []string{metrics.UrlLabel, req.URL.String(), metrics.ReasonLabel, reason, metrics.CodeLabel, code, metrics.EventLabel, event}
		fmt.Print(deliveryCounterLabels)
		// v1.droppedMessage.With(metrics.UrlLabel, req.URL.String(), metrics.ReasonLabel, reason).Add(1)
		logger.Error("Dropped Network Error", zap.Error(err))
		return err
	} else {
		// Report Result
		code = strconv.Itoa(resp.StatusCode)

		// read until the response is complete before closing to allow
		// connection reuse
		if resp.Body != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		deliveryCounterLabels = []string{metrics.UrlLabel, req.URL.String(), metrics.ReasonLabel, reason, metrics.CodeLabel, code, metrics.EventLabel, event}
	}
	fmt.Print(deliveryCounterLabels)

	//TODO: do we add deliveryCounter to webhook metrics and remove from sink sender?
	// v1.deliveryCounter.With(prometheus.Labels{deliveryCounterLabels}).Add(1.0)
	logger.Debug("event sent-ish", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination), zap.String(metrics.CodeLabel, code), zap.String(metrics.UrlLabel, req.URL.String()))
	return nil
}

func (v1 *WebhookV1) addRunner(request *http.Request, event string) retry.Runner[*http.Response] {
	runner, _ := retry.NewRunner[*http.Response](
		retry.WithPolicyFactory[*http.Response](retry.Config{
			Interval:   v1.deliveryInterval,
			MaxRetries: v1.deliveryRetries,
		}),
		retry.WithOnAttempt[*http.Response](v1.onAttempt(request, event)),
	)
	return runner
}

func (v1 *WebhookV1) updateRequest(urls *ring.Ring) func(*http.Request) *http.Request {
	return func(request *http.Request) *http.Request {
		urls = urls.Next()
		tmp, err := url.Parse(urls.Value.(string))
		if err != nil {
			v1.logger.Error("failed to update url", zap.String(metrics.UrlLabel, urls.Value.(string)), zap.Error(err))
			//TODO: do we add droppedMessage metric to webhook and remove from sink sender?
			// v1.droppedMessage.With(metrics.UrlLabel, request.URL.String(), metrics.ReasonLabel, metrics.UpdateRequestURLFailedReason).Add(1)
		}
		request.URL = tmp
		return request
	}
}

func (v1 *WebhookV1) onAttempt(request *http.Request, event string) retry.OnAttempt[*http.Response] {

	return func(attempt retry.Attempt[*http.Response]) {
		if attempt.Retries > 0 {
			fmt.Print(event)
			// s.deliveryRetryCounter.With(prometheus.Labels{UrlLabel: v1.id, EventLabel: event}).Add(1.0)
			v1.logger.Debug("retrying HTTP transaction", zap.String(metrics.UrlLabel, request.URL.String()), zap.Error(attempt.Err), zap.Int("retry", attempt.Retries+1), zap.Int("statusCode", attempt.Result.StatusCode))
		}

	}
}

func (k *Kafka) Update(id, topic string, retries int, servers []string, logger *zap.Logger) error {
	k.id = id
	k.topic = topic
	k.brokerAddr = append(k.brokerAddr, servers...)
	k.logger = logger

	config := sarama.NewConfig()
	//TODO: this is basic set up for now - will need to add more options to config
	//once we know what we are allowing users to send

	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = retries //should we be using retryhint for this?
	k.config = config
	// Create a new Kafka producer
	producer, err := sarama.NewSyncProducer(k.brokerAddr, config)
	if err != nil {
		k.logger.Error("Could not create Kafka producer", zap.Error(err))
		return err
	}

	k.producer = producer
	return nil
}

func (k Kafkas) Send(secret string, acceptType string, msg *wrp.Message) error {
	//TODO: discuss with wes and john the default hashing logic
	//for now: when no hash is given we will just loop through all the kafkas
	var errs error
	for _, kafka := range k {
		err := kafka.send(secret, acceptType, msg)
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func (k *Kafka) send(secret string, acceptType string, msg *wrp.Message) error {

	defer func() {
		if r := recover(); nil != r {
			// s.droppedPanic.Add(1.0)
			//TODO: should we be using the RegistrationV2 id for this (canonical_name)
			//or should we have an id for the specific kafka instance that failed?
			k.logger.Error("goroutine send() panicked", zap.String("id", k.id), zap.Any("panic", r))
		}
		// s.workers.Release()
		// s.currentWorkersGauge.Add(-1.0)
	}()

	payload := msg.Payload
	body := payload

	// Use the internal content type unless the accept type is wrp
	contentType := msg.ContentType
	switch acceptType {
	case "wrp", wrp.MimeTypeMsgpack, wrp.MimeTypeWrp:
		// WTS - We should pass the original, raw WRP event instead of
		// re-encoding it.
		contentType = wrp.MimeTypeMsgpack
		//TODO: do we want to use the wrp encoder or the sarama encoder?
		buffer := bytes.NewBuffer([]byte{})
		encoder := wrp.NewEncoder(buffer, wrp.Msgpack)
		encoder.Encode(msg)
		body = buffer.Bytes()
	}

	id, _ := wrp.ParseDeviceID(msg.Source)
	var sig string
	if secret != "" {
		s := hmac.New(sha1.New, []byte(secret))
		s.Write(body)
		sig = fmt.Sprintf("sha1=%s", hex.EncodeToString(s.Sum(nil)))
	}
	eventHeader := strings.TrimPrefix(msg.Destination, "event:")

	// Create a Kafka message
	kafkaMsg := &sarama.ProducerMessage{
		Topic: k.topic,
		Key:   nil,
		Value: sarama.ByteEncoder(msg.Payload),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("X-Webpa-Device-Id"),
				Value: []byte(id),
			},
			{
				Key:   []byte("X-Webpa-Device-Name"),
				Value: []byte(id),
			},
			{
				Key:   []byte("X-Webpa-Event"),
				Value: []byte(eventHeader),
			},
			{
				Key:   []byte("X-Webpa-Transaction-Id"),
				Value: []byte(msg.TransactionUUID),
			},
			{
				Key:   []byte("X-Webpa-Signature"),
				Value: []byte(sig),
			},
			{
				Key:   []byte("Content-Type"),
				Value: []byte(contentType),
			},
		},
	}

	//TODO: need to determine if all of these headers are necessary
	AddMessageHeaders(kafkaMsg, msg)

	// Send the message to Kafka
	partition, offset, err := k.producer.SendMessage(kafkaMsg)
	defer k.producer.Close()
	if err != nil {
		k.logger.Error("Failed to send message to Kafka", zap.Error(err))
		return err
	}

	k.logger.Debug("Message sent to Kafka",

		zap.String("Topic", kafkaMsg.Topic),
		zap.Int32("Partition", partition),
		zap.Int64("Offset", offset),
	)

	return nil

}

func AddMessageHeaders(kafkaMsg *sarama.ProducerMessage, m *wrp.Message) {
	kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
		Key:   []byte(wrphttp.MessageTypeHeader),
		Value: []byte(m.Type.FriendlyName()),
	})

	if len(m.Source) > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.SourceHeader),
			Value: []byte(m.Source),
		})
	}

	if len(m.Destination) > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.DestinationHeader),
			Value: []byte(m.Destination),
		})
	}

	if len(m.TransactionUUID) > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.TransactionUuidHeader),
			Value: []byte(m.TransactionUUID),
		})
	}

	if m.Status != nil {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.StatusHeader),
			Value: []byte(strconv.FormatInt(*m.Status, 10)),
		})
	}

	if m.RequestDeliveryResponse != nil {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.RequestDeliveryResponseHeader),
			Value: []byte(strconv.FormatInt(*m.RequestDeliveryResponse, 10)),
		})
	}

	// TODO Remove along with `IncludeSpans`
	//this todo was copied over when i copied the wrphttp.AddMessageHeaders  
	// nolint:staticcheck
	if m.IncludeSpans != nil {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.IncludeSpansHeader),
			Value: []byte(strconv.FormatBool(*m.IncludeSpans)),
		})
	}

	for _, s := range m.Spans {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.SpanHeader),
			Value: []byte(strings.Join(s, ",")),
		})
	}

	if len(m.Accept) > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.AcceptHeader),
			Value: []byte(m.Accept),
		})
	}

	if len(m.Path) > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.PathHeader),
			Value: []byte(m.Path),
		})
	}

	var bufStrings []string
	for k, v := range m.Metadata {
		// perform k + "=" + v more efficiently
		buf := bytes.Buffer{}
		buf.WriteString(k)
		buf.WriteString("=")
		buf.WriteString(v)
		bufStrings = append(bufStrings, buf.String())
	}

	kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
		Key:   []byte(wrphttp.MetadataHeader),
		Value: []byte(strings.Join(bufStrings, ",")),
	})

	kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
		Key:   []byte(wrphttp.PartnerIdHeader),
		Value: []byte(strings.Join(m.PartnerIDs, ",")),
	})

	if len(m.SessionID) > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.SessionIdHeader),
			Value: []byte(m.SessionID),
		})
	}

	kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
		Key:   []byte(wrphttp.HeadersHeader),
		Value: []byte(strings.Join(m.Headers, ",")),
	})

	if len(m.ServiceName) > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.ServiceNameHeader),
			Value: []byte(m.ServiceName),
		})
	}

	if len(m.URL) > 0 {
		kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
			Key:   []byte(wrphttp.URLHeader),
			Value: []byte(m.URL),
		})
	}
}
