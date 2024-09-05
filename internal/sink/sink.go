// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

import (
	"bytes"
	"container/ring"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	Update(ancla.Register) error
	Send(*ring.Ring, string, string, *wrp.Message) error
}

type WebhookV1 struct {
	id               string
	deliveryInterval time.Duration
	deliveryRetries  int
	logger           *zap.Logger
	//TODO: need to determine best way to add client and client middleware to WebhooV1
	// client           http.Client
	// clientMiddleware func(http.Client) http.Client
}

type Kafkas []*Kafka
type Kafka struct {
	id         string
	logger     *zap.Logger
	brokerAddr []string
	topic      string
	config     *sarama.Config
	producer   sarama.SyncProducer
}

func NewSink(c Config, logger *zap.Logger, listener ancla.Register) Sink {
	var sink Sink
	switch l := listener.(type) {
	case *ancla.RegistryV1:
		sink = &WebhookV1{
			id:               l.GetId(),
			deliveryInterval: c.DeliveryInterval,
			deliveryRetries:  c.DeliveryRetries,
			logger:           logger,
		}
		return sink
	case *ancla.RegistryV2:
		var sink Kafkas
		for _, k := range l.Registration.Kafkas {
			kafka := &Kafka{
				id:         l.Registration.CanonicalName,
				brokerAddr: k.BootstrapServers,
				topic:      "test",
			}

			//TODO: this is basic set up for now - will need to add more options to config
			//once we know what we are allowing users to send
			kafka.config.Producer.Return.Successes = true			
			kafka.config.Producer.RequiredAcks = sarama.WaitForAll
			kafka.config.Producer.Retry.Max = c.DeliveryRetries //should we be using retryhint for this?

			// Create a new Kafka producer
			producer, err := sarama.NewSyncProducer(kafka.brokerAddr, config)
			if err != nil {
				kafka.logger.Error("Could not create Kafka producer", zap.Error(err))
				return nil
			}
			defer producer.Close()

			kafka.producer = producer
			sink = append(sink, kafka)
		}

	default:
		return nil
	}
	return sink
}

func (v1 *WebhookV1) Update(l ancla.Register) (err error) {
	//TODO: is there anything else that needs to be done for this?
	//do we need to return an error?
	v1.id = l.GetId()
	return nil
}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (v1 *WebhookV1) Send(urls *ring.Ring, secret, acceptType string, msg *wrp.Message) error {
	defer func() {
		if r := recover(); nil != r {
			// s.droppedPanic.Add(1.0)
			v1.logger.Error("goroutine send() panicked", zap.String("id", v1.id), zap.Any("panic", r))
		}
		// s.workers.Release()
		// s.currentWorkersGauge.Add(-1.0)
	}()

	//TODO: is there a reason we are setting it up like this?
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
	if err != nil {
		// Report drop
		// s.droppedInvalidConfig.Add(1.0)
		v1.logger.Error("Invalid URL", zap.String(metrics.UrlLabel, urls.Value.(string)), zap.String("id", v1.id), zap.Error(err))
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
		retryhttp.WithRequesters(v1.updateRequest(urls)),
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

func (k Kafkas) Update(l ancla.Register) error {
	return nil
}

// TODO: probably get rid of urls
func (k Kafkas) Send(urls *ring.Ring, secret string, acceptType string, msg *wrp.Message) error {
	//TODO: is this how we want to set this up?
	//or do we want to only send to specific kafkas in the list based on an id
	for _, kafka := range k {
		err := kafka.send(secret, acceptType, msg)
		return err
	}
	return nil
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

	//add more headers
	//TODO: need to determine if all of these headers are necessary
	AddMessageHeaders(kafkaMsg, msg)

	// Send the message to Kafka
	partition, offset, err := k.producer.SendMessage(kafkaMsg)
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
