// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"container/ring"

	"github.com/xmidt-org/wrp-go/v3"
)

type Dispatcher interface {
	QueueOverflow()
	Send(urls *ring.Ring, secret, acceptType string, msg *wrp.Message)
}

func DispatcherFactory(webhook bool, obs *CaduceusOutboundSender) Dispatcher {

	if webhook {
		return &WebhookDispatcher{
			obs: obs,
		}
	}

	// TODO
	return &WebhookDispatcher{
		obs: obs,
	}

}
