// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"container/ring"
	"strings"

	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/xmidt-org/caduceus/internal/stream"

	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

type StreamDispatcher struct {
	obs    *CaduceusOutboundSender
	sender stream.StreamSender
}

func NewStreamDispatcher(obs *CaduceusOutboundSender) (Dispatcher, error) {

	// TODO - sender should hit alternatives (east vs west) if kinesis is down
	sender, err := stream.New(obs.streamVersion, obs.streamSender, obs.logger)
	if err != nil {
		obs.logger.Error("error creating stream sender", zap.Error(err))
		return nil, err
	}

	return &StreamDispatcher{
		obs:    obs,
		sender: sender,
	}, nil

}

// Note - first go around we will not batch the records
// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (d *StreamDispatcher) Send(urls *ring.Ring, secret, acceptType string, msg *wrp.Message) {
	defer func() {
		if r := recover(); nil != r {
			d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: d.obs.id, reasonLabel: DropsDueToPanic}).Add(1.0)
			d.obs.logger.Error("stream goroutine send() panicked", zap.String("id", d.obs.id), zap.Any("panic", r))
			// don't silence the panic
			panic(r)
		}

		d.obs.workers.Release()
		d.obs.metrics.currentWorkersGauge.With(prometheus.Labels{urlLabel: d.obs.id}).Add(-1.0)
	}()

	url := urls.Value.(string)

	d.sendToEndpoint(url, msg)
}

func (d *StreamDispatcher) sendToEndpoint(url string, msg *wrp.Message) {
	// Send it
	d.obs.logger.Debug("attempting to send event", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))

	msgs := []*wrp.Message{msg}
	failedRecordCount, err := d.sender.OnEvent(msgs, url)

	eventType := strings.ToValidUTF8(msg.FindEventStringSubMatch(), "")
	var deliveryCounterLabels prometheus.Labels
	code := messageDroppedCode
	reason := "no_err"
	l := d.obs.logger
	if err != nil {
		reason = "send_error"
		d.obs.logger.Error("error writing to stream", zap.String(reasonLabel, reason), zap.Error(err))
		deliveryCounterLabels = prometheus.Labels{urlLabel: url, reasonLabel: reason, codeLabel: code, eventLabel: eventType}
		d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: url, reasonLabel: reason}).Add(1)
	} else if failedRecordCount > 0 {
		reason = "some_records_failed"
		d.obs.logger.Error("some records failed to write to stream", zap.String(reasonLabel, reason), zap.Int("failedRecordCount", failedRecordCount))
		deliveryCounterLabels = prometheus.Labels{urlLabel: url, reasonLabel: reason, codeLabel: code, eventLabel: eventType}
		d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: url, reasonLabel: reason}).Add(1)
	} else {
		deliveryCounterLabels = prometheus.Labels{urlLabel: url, reasonLabel: reason, codeLabel: "200", eventLabel: eventType}
	}

	d.obs.metrics.deliveryCounter.With(deliveryCounterLabels).Add(1.0)
	l.Debug("event sent-ish", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination), zap.String("code", code), zap.String("url", url))
}

// queueOverflow handles the logic of what to do when a queue overflows:
// cutting off the stream for a time (TODO - should we send cutoff message to the stream?)
func (d *StreamDispatcher) QueueOverflow() {
	d.obs.mutex.Lock()
	if time.Now().Before(d.obs.dropUntil) {
		d.obs.mutex.Unlock()
		return
	}
	d.obs.dropUntil = time.Now().Add(d.obs.cutOffPeriod)
	d.obs.metrics.dropUntilGauge.With(prometheus.Labels{urlLabel: d.obs.id}).Set(float64(d.obs.dropUntil.Unix()))
	d.obs.mutex.Unlock()

	d.obs.metrics.cutOffCounter.With(prometheus.Labels{urlLabel: d.obs.id}).Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	d.obs.Empty(d.obs.metrics.droppedMessage.With(prometheus.Labels{urlLabel: d.obs.id, reasonLabel: "cut_off"}))
}
