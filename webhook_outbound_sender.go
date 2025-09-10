// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"container/ring"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

const Secret = "XxxxxX"

type webhookOutboundSender struct {
	obs *CaduceusOutboundSender
}

// TODO - you should not be able to create it any other way
func NewWebhookOutboundSender(obs *CaduceusOutboundSender) (OutboundSender, error) {
	dispatcher := NewWebhookDispatcher(obs)

	whSender := &webhookOutboundSender{
		obs: obs,
	}

	whSender.Dispatch(dispatcher)

	return whSender, nil
}

func (s *webhookOutboundSender) RetiredSince() time.Time {
	s.obs.mutex.RLock()
	deliverUntil := s.obs.deliverUntil
	s.obs.mutex.RUnlock()
	return deliverUntil
}

func (s *webhookOutboundSender) Dispatch(d Dispatcher) {
	s.obs.Dispatch(d)
}

func (s *webhookOutboundSender) Queue(msg *wrp.Message) {
	s.obs.Queue(msg)
}

func (s *webhookOutboundSender) Shutdown(gentle bool) {
	s.obs.Shutdown(gentle)
}

func (s *webhookOutboundSender) Update(wh ancla.InternalWebhook) (err error) {
	// Validate the failure URL, if present
	if wh.Webhook.FailureURL != "" {
		if _, err = url.ParseRequestURI(wh.Webhook.FailureURL); err != nil {
			return
		}
	}

	// Create and validate the event regex objects
	// nolint:prealloc
	var events []*regexp.Regexp
	for _, event := range wh.Webhook.Events {
		var re *regexp.Regexp
		if re, err = regexp.Compile(event); err != nil {
			return
		}

		events = append(events, re)
	}
	if len(events) < 1 {
		err = errors.New("events must not be empty")
		return
	}

	// Create the matcher regex objects
	matcher := []*regexp.Regexp{}
	for _, item := range wh.Webhook.Matcher.DeviceID {
		if item == ".*" {
			// Match everything - skip the filtering
			matcher = []*regexp.Regexp{}
			break
		}

		var re *regexp.Regexp
		if re, err = regexp.Compile(item); err != nil {
			err = fmt.Errorf("invalid matcher item: '%s'", item)
			return
		}
		matcher = append(matcher, re)
	}

	// Validate the various urls
	urlCount := len(wh.Webhook.Config.AlternativeURLs)
	for i := 0; i < urlCount; i++ {
		_, err = url.Parse(wh.Webhook.Config.AlternativeURLs[i])
		if err != nil {
			s.obs.logger.Error("failed to update url", zap.Any("url", wh.Webhook.Config.AlternativeURLs[i]), zap.Error(err))
			return
		}
	}

	s.obs.metrics.renewalTimeGauge.With(prometheus.Labels{urlLabel: s.obs.id}).Set(float64(time.Now().Unix()))

	// write/update obs
	s.obs.mutex.Lock()

	s.obs.listener = wh

	s.obs.failureMsg.Original = wh
	// Don't share the secret with others when there is an error.
	s.obs.failureMsg.Original.Webhook.Config.Secret = Secret

	s.obs.listener.Webhook.FailureURL = wh.Webhook.FailureURL
	s.obs.deliverUntil = wh.Webhook.Until
	s.obs.metrics.deliverUntilGauge.With(prometheus.Labels{urlLabel: s.obs.id}).Set(float64(s.obs.deliverUntil.Unix()))

	s.obs.events = events

	s.obs.metrics.deliveryRetryMaxGauge.With(prometheus.Labels{urlLabel: s.obs.id}).Set(float64(s.obs.deliveryRetries))

	// if matcher list is empty set it nil for Queue() logic
	s.obs.matcher = nil
	if 0 < len(matcher) {
		s.obs.matcher = matcher
	}

	if urlCount == 0 {
		s.obs.urls = ring.New(1)
		s.obs.urls.Value = s.obs.id
	} else {
		r := ring.New(urlCount)
		for i := 0; i < urlCount; i++ {
			r.Value = wh.Webhook.Config.AlternativeURLs[i]
			r = r.Next()
		}
		s.obs.urls = r
	}

	// Randomize where we start so all the instances don't synchronize
	// not sure why lint is complaining about below line, it is not using crypto/rand
	r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	offset := r.Intn(s.obs.urls.Len())
	for 0 < offset {
		s.obs.urls = s.obs.urls.Next()
		offset--
	}

	// Update this here in case we make this configurable later
	s.obs.metrics.maxWorkersGauge.With(prometheus.Labels{urlLabel: s.obs.id}).Set(float64(s.obs.maxWorkers))

	s.obs.mutex.Unlock()

	return
}
