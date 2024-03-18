// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"container/ring"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/xmidt-org/retry"
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

// CaduceusOutboundSender is the outbound sender object.
type SinkSender struct {
	customPIDs       []string
	client           Client
	clientMiddleware func(Client) Client
	sink             SinkMiddleware
}

func newSinkSender(sw *SinkWrapper, listener ListenerStub) (s Sender, err error) {
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

	id := listener.Registration.GetId()

	sinkSender := &SinkSender{
		client:           sw.client,
		clientMiddleware: sw.clientMiddleware,
	}

	cs := CommonSink{
		id:                id,
		maxWorkers:        sw.config.NumWorkersPerSender,
		deliveryRetries:   sw.config.DeliveryRetries,
		logger:            sw.logger,
		queueSize:         sw.config.QueueSizePerSender,
		cutOffPeriod:      sw.config.CutOffPeriod,
		disablePartnerIDs: sw.config.DisablePartnerIDs,
		customPIDs:        sw.config.CustomPIDs,
		deliveryInterval:  sw.config.DeliveryInterval,
	}
	CreateFailureMessage(sw.config, listener, sinkSender.sink)
	CreateSinkMetrics(sw.metrics, id, sinkSender.sink)

	//TODO: need to figure out how to set this up
	// Don't share the secret with others when there is an error.
	// sinkSender.failureMsg.Original.Webhook.Config.Secret = "XxxxxX"
	sinkSender.sink.SetCommonSink(cs)

	if err = sinkSender.Update(listener); nil != err {
		return
	}

	go sinkSender.sink.dispatcher()

	s = sinkSender

	return
}

// Update applies user configurable values for the outbound sender when a
// webhook is registered
// TODO: commenting out for now until argus/ancla dependency issue is fixed
func (s *SinkSender) Update(wh ListenerStub) (err error) {
	return
}

// Shutdown causes the CaduceusOutboundSender to stop its activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (s *SinkSender) Shutdown(gentle bool) {

}

func (s *SinkSender) UpdateR2(v2 *RegistrationV2) error {
	//TODO: ADD CODE
	return nil
}

// RetiredSince returns the time the CaduceusOutboundSender retired (which could be in
// the future).
func (s *SinkSender) RetiredSince() (t time.Time) {
	return
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

}

func (s *SinkSender) isValidTimeWindow(now, dropUntil, deliverUntil time.Time) (b bool) {
	return
}

// Empty is called on cutoff or shutdown and swaps out the current queue for
// a fresh one, counting any current messages in the queue as dropped.
// It should never close a queue, as a queue not referenced anywhere will be
// cleaned up by the garbage collector without needing to be closed.
func (s *SinkSender) Empty(droppedCounter prometheus.Counter) {

}

func (s *SinkSender) dispatcher() {

}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (s *SinkSender) send(urls *ring.Ring, secret, acceptType string, msg *wrp.Message) {
}

// queueOverflow handles the logic of what to do when a queue overflows:
// cutting off the webhook for a time and sending a cut off notification
// to the failure URL.
func (s *SinkSender) queueOverflow() {

}

func (s *SinkSender) addRunner(request *http.Request, event string) (f retry.Runner[*http.Response]) {
	//TODO: need to handle error
	return

}

func (s *SinkSender) updateRequest(urls *ring.Ring) (f func(*http.Request) *http.Request) {
	return
}

func (s *SinkSender) onAttempt(request *http.Request, event string) (f retry.OnAttempt[*http.Response]) {
	return
}
