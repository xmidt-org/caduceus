// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"time"

	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/wrp-go/v3"
)

type StreamSenderConfig struct {
	// these senders are not really webhooks, but we can treat them as such
	OutboundStreamSenders []ancla.InternalWebhook
}

type streamOutboundSender struct {
	obs *CaduceusOutboundSender
}

// TODO -  you should not be able to create it any other way
func NewStreamOutboundSender(obs *CaduceusOutboundSender) (OutboundSender, error) {
	dispatcher, err := NewStreamDispatcher(obs)
	if err != nil {
		return nil, err
	}

	sender := &streamOutboundSender{
		obs: obs,
	}

	sender.Dispatch(dispatcher)

	return sender, nil
}

func (s *streamOutboundSender) RetiredSince() time.Time {
	// stream senders never expire
	deliverUntil := time.Now().UTC().Add(time.Hour * 24)
	s.obs.mutex.RLock()
	s.obs.deliverUntil = deliverUntil
	s.obs.mutex.RUnlock()
	return deliverUntil
}

func (s *streamOutboundSender) Dispatch(d Dispatcher) {
	s.obs.Dispatch(d)
}

func (s *streamOutboundSender) Queue(msg *wrp.Message) {
	s.obs.Queue(msg)
}

func (s *streamOutboundSender) Shutdown(gentle bool) {
	s.obs.Shutdown(gentle)
}

func (s *streamOutboundSender) Update(wh ancla.InternalWebhook) error {
	// this is not truly a webhook and is built statically from config at startup
	s.obs.logger.Info("Update is NOOP for stream senders")
	return nil
}
