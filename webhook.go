// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xmidt-org/ancla/model"
	"github.com/xmidt-org/ancla/schema"
	"go.uber.org/zap"
)

// The types below keep the ancla 0.3.x JSON layout exactly. They are what the
// sender works with, and the cut-off notification embeds InternalWebhook, so
// changing a tag here changes a message other services read.

// DeliveryConfig is where and how events are delivered.
type DeliveryConfig struct {
	URL             string   `json:"url"`
	ContentType     string   `json:"content_type"`
	Secret          string   `json:"secret,omitempty"`
	AlternativeURLs []string `json:"alt_urls,omitempty"`
}

// MetadataMatcherConfig selects events by device.
type MetadataMatcherConfig struct {
	DeviceID []string `json:"device_id"`
}

// Webhook is one registration as the sender uses it.
type Webhook struct {
	Address    string                `json:"registered_from_address"`
	Config     DeliveryConfig        `json:"config"`
	FailureURL string                `json:"failure_url"`
	Events     []string              `json:"events"`
	Matcher    MetadataMatcherConfig `json:"matcher,omitempty"`
	Duration   time.Duration         `json:"duration"`
	Until      time.Time             `json:"until"`
}

// InternalWebhook is a Webhook with the partner IDs that own it.
type InternalWebhook struct {
	PartnerIDs []string
	Webhook    Webhook
}

// itemToWebhooks decodes an Argus item written by any ancla version.
func itemToWebhooks(item model.Item) ([]InternalWebhook, error) {
	manifest, err := schema.ItemToSchema(item)
	if err != nil {
		return nil, err
	}

	switch m := manifest.(type) {
	case *schema.ManifestV1:
		return []InternalWebhook{fromSchemaV1(m)}, nil
	case *schema.ManifestV2:
		return fromSchemaV2(m)
	default:
		return nil, fmt.Errorf("unsupported manifest type %T", manifest)
	}
}

func fromSchemaV1(m *schema.ManifestV1) InternalWebhook {
	r := m.Registration
	return InternalWebhook{
		PartnerIDs: m.PartnerIDs,
		Webhook: Webhook{
			Address: r.Address,
			Config: DeliveryConfig{
				URL:             r.Config.ReceiverURL,
				ContentType:     r.Config.ContentType,
				Secret:          r.Config.Secret,
				AlternativeURLs: r.Config.AlternativeURLs,
			},
			FailureURL: r.FailureURL,
			Events:     r.Events,
			Matcher:    MetadataMatcherConfig{DeviceID: r.Matcher.DeviceID},
			Duration:   time.Duration(r.Duration),
			Until:      r.Until,
		},
	}
}

// fromSchemaV2 maps each v2 webhook sink to one sender registration.  Kafka
// sinks, batching and the secret hash algorithm have no equivalent here and
// are ignored.  A registration that depends on something the sender cannot do
// (hash distribution, DNS SRV receivers, matching on fields other than
// destination and source) is rejected, because delivering it anyway would
// send events the subscriber did not ask for.
func fromSchemaV2(m *schema.ManifestV2) ([]InternalWebhook, error) {
	r := m.Registration

	if r.Hash.Field != "" {
		return nil, errors.New("hash distribution is not supported")
	}

	var events, devices []string
	for _, fr := range r.Matcher {
		switch strings.ToLower(fr.Field) {
		case "destination":
			if events != nil {
				return nil, errors.New("more than one destination matcher is not supported")
			}
			events = []string{fr.Regex}
		case "source":
			if devices != nil {
				return nil, errors.New("more than one source matcher is not supported")
			}
			devices = []string{fr.Regex}
		default:
			return nil, fmt.Errorf("matcher on field %q is not supported", fr.Field)
		}
	}
	if events == nil {
		events = []string{".*"}
	}

	if len(r.Webhooks) == 0 {
		return nil, errors.New("registration has no webhook sinks")
	}

	hooks := make([]InternalWebhook, 0, len(r.Webhooks))
	for _, w := range r.Webhooks {
		if len(w.ReceiverURLs) == 0 {
			return nil, errors.New("webhook sink without receiver_urls is not supported")
		}
		hooks = append(hooks, InternalWebhook{
			PartnerIDs: m.PartnerIds,
			Webhook: Webhook{
				Address: r.Address,
				Config: DeliveryConfig{
					URL:             w.ReceiverURLs[0],
					ContentType:     w.Accept,
					Secret:          w.Secret,
					AlternativeURLs: w.ReceiverURLs[1:],
				},
				FailureURL: r.FailureURL,
				Events:     events,
				Matcher:    MetadataMatcherConfig{DeviceID: devices},
				Until:      r.Expires,
			},
		})
	}

	return hooks, nil
}

// itemsToWebhooks decodes every item it can.  An item that cannot be decoded is
// logged and left out so that one bad registration does not stall the rest.
func itemsToWebhooks(items []model.Item, logger *zap.Logger) []InternalWebhook {
	hooks := make([]InternalWebhook, 0, len(items))
	for _, item := range items {
		decoded, err := itemToWebhooks(item)
		if err != nil {
			logger.Warn("Skipping webhook item that could not be used", zap.String("id", item.ID), zap.Error(err))
			continue
		}
		hooks = append(hooks, decoded...)
	}

	return hooks
}
