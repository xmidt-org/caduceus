// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"container/ring"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	gokitprometheus "github.com/go-kit/kit/metrics/prometheus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/webpa-common/v2/xhttp"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
	"go.uber.org/zap"
)

type WebhookDispatcher struct {
	obs *CaduceusOutboundSender
}

// needed - wg, queue, httpclient, urls, metrics
func NewWebhookDispatcher(obs *CaduceusOutboundSender) Dispatcher {
	return &WebhookDispatcher{
		obs: obs,
	}

}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (d *WebhookDispatcher) Send(urls *ring.Ring, secret, acceptType string, msg *wrp.Message) {
	defer func() {
		if r := recover(); nil != r {
			d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: d.obs.id, reasonLabel: DropsDueToPanic}).Add(1.0)
			d.obs.logger.Error("goroutine send() panicked", zap.String("id", d.obs.id), zap.Any("panic", r))
			// don't silence the panic
			panic(r)
		}

		d.obs.workers.Release()
		d.obs.metrics.currentWorkersGauge.With(prometheus.Labels{urlLabel: d.obs.id}).Add(-1.0)
	}()

	payload := msg.Payload
	body := payload
	var payloadReader *bytes.Reader

	// Use the internal content type unless the accept type is wrp
	contentType := msg.ContentType
	switch acceptType {
	// Not sure if we will break clients here if we lint this
	case "wrp", wrp.MimeTypeMsgpack, wrp.MimeTypeWrp: //nolint:staticcheck
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
		d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: d.obs.id, reasonLabel: "invalid_config"}).Add(1.0)
		d.obs.logger.Error("Invalid URL", zap.String("url", urls.Value.(string)), zap.String("id", d.obs.id), zap.Error(err))
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
	// Deprecated: X-Webpa-Device-Id should only be used for backwards compatibility.
	// Use X-Webpa-Source instead.
	req.Header.Set("X-Webpa-Device-Id", string(id))
	// Deprecated: X-Webpa-Device-Name should only be used for backwards compatibility.
	// Use X-Webpa-Source instead.
	req.Header.Set("X-Webpa-Device-Name", string(id))
	req.Header.Set("X-Webpa-Source", msg.Source)
	req.Header.Set("X-Webpa-Destination", msg.Destination)

	// Apply the secret

	if secret != "" {
		s := hmac.New(sha1.New, []byte(secret))
		s.Write(body)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(s.Sum(nil)))
		req.Header.Set("X-Webpa-Signature", sig)
	}

	// since eventType is only used to enrich metrics and logging, remove invalid UTF-8 characters from the URL
	eventType := strings.ToValidUTF8(msg.FindEventStringSubMatch(), "")

	retryOptions := xhttp.RetryOptions{
		Logger:   d.obs.logger,
		Retries:  d.obs.deliveryRetries,
		Interval: d.obs.deliveryInterval,
		Counter:  gokitprometheus.NewCounter(d.obs.metrics.deliveryRetryCounter.MustCurryWith(prometheus.Labels{urlLabel: d.obs.id, eventLabel: eventType})),
		// Always retry on failures up to the max count.
		ShouldRetry:       xhttp.ShouldRetry,
		ShouldRetryStatus: xhttp.RetryCodes,
	}

	// update subsequent requests with the next url in the list upon failure
	retryOptions.UpdateRequest = func(request *http.Request) {
		urls = urls.Next()
		tmp, err := url.Parse(urls.Value.(string))
		if err != nil {
			d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: req.URL.String(), reasonLabel: updateRequestURLFailedReason}).Add(1)
			d.obs.logger.Error("failed to update url", zap.String("url", urls.Value.(string)), zap.Error(err))
			return
		}
		request.URL = tmp
	}

	// Send it
	d.obs.logger.Debug("attempting to send event", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))

	// do not know how to address bodyclose on the below line
	retryer := xhttp.RetryTransactor(retryOptions, d.obs.httpSender.Do) //nolint:bodyclose
	client := d.obs.clientMiddleware(doerFunc(retryer))
	resp, err := client.Do(req)
	defer func() {
		if resp.Body != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	var deliveryCounterLabels prometheus.Labels
	code := messageDroppedCode
	reason := noErrReason
	l := d.obs.logger
	if err != nil {
		// Report failure
		reason = getDoErrReason(err)
		if resp != nil {
			code = strconv.Itoa(resp.StatusCode)
		}

		l = d.obs.logger.With(zap.String(reasonLabel, reason), zap.Error(err))
		deliveryCounterLabels = prometheus.Labels{urlLabel: req.URL.String(), reasonLabel: reason, codeLabel: code, eventLabel: eventType}
		d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: req.URL.String(), reasonLabel: reason}).Add(1)
	} else {
		// Report Result
		code = strconv.Itoa(resp.StatusCode)

		deliveryCounterLabels = prometheus.Labels{urlLabel: req.URL.String(), reasonLabel: reason, codeLabel: code, eventLabel: eventType}
	}

	d.obs.metrics.deliveryCounter.With(deliveryCounterLabels).Add(1.0)
	l.Debug("event sent-ish", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination), zap.String("code", code), zap.String("url", req.URL.String()))
}

// queueOverflow handles the logic of what to do when a queue overflows:
// cutting off the webhook for a time and sending a cut off notification
// to the failure URL.
func (d *WebhookDispatcher) QueueOverflow() {
	d.obs.mutex.Lock()
	if time.Now().Before(d.obs.dropUntil) {
		d.obs.mutex.Unlock()
		return
	}
	d.obs.dropUntil = time.Now().Add(d.obs.cutOffPeriod)
	d.obs.metrics.dropUntilGauge.With(prometheus.Labels{urlLabel: d.obs.id}).Set(float64(d.obs.dropUntil.Unix()))
	secret := d.obs.listener.Webhook.Config.Secret
	failureMsg := d.obs.failureMsg
	failureURL := d.obs.listener.Webhook.FailureURL
	d.obs.mutex.Unlock()

	d.obs.metrics.cutOffCounter.With(prometheus.Labels{urlLabel: d.obs.id}).Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	d.obs.Empty(d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: d.obs.id, reasonLabel: "cut_off"}))

	msg, err := json.Marshal(failureMsg)
	if err != nil {
		d.obs.logger.Error("Cut-off notification json.Marshal failed", zap.Any("failureMessage", d.obs.failureMsg), zap.String("for", d.obs.id), zap.Error(err))
		return
	}

	// if no URL to send cut off notification to, do nothing
	if failureURL == "" {
		return
	}

	// Send a "you've been cut off" warning message
	payload := bytes.NewReader(msg)
	req, err := http.NewRequest("POST", failureURL, payload)
	if err != nil {
		// Failure
		d.obs.logger.Error("Unable to send cut-off notification", zap.String("notification",
			failureURL), zap.String("for", d.obs.id), zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", wrp.MimeTypeJson)

	if secret != "" {
		// we can't break the existing clients
		h := hmac.New(sha1.New, []byte(secret)) //nolint:gosec
		h.Write(msg)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
		req.Header.Set("X-Webpa-Signature", sig)
	}

	resp, err := d.obs.httpSender.Do(req)
	if err != nil {
		// Failure
		d.obs.logger.Error("Unable to send cut-off notification", zap.String("notification", failureURL), zap.String("for", d.obs.id), zap.Error(err))
		return
	}

	if resp == nil {
		// Failure
		d.obs.logger.Error("Unable to send cut-off notification, nil response", zap.String("notification", failureURL))
		return
	}

	// Success

	if resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

	}
}
