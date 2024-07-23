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
