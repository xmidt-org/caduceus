// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xmidt-org/ancla/model"
	"github.com/xmidt-org/ancla/schema"
	webhook "github.com/xmidt-org/webhook-schema"
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

// Wrapper keys that identify which ancla version stored an item.
const (
	legacyItemKey   = "Webhook"
	schemaV1ItemKey = "wrp_event_stream_schema_v1"
	schemaV2ItemKey = "wrp_event_stream_schema_v2"
)

var errUnrecognizedItem = errors.New("item is not a recognized webhook registration")

// itemToWebhooks decodes an Argus item written by ancla 0.3.x (legacy), or by
// ancla 0.5.x (schema v1 or v2).  ancla's own decoder is not used because it
// reads a legacy item as an empty v1 registration without reporting an error.
func itemToWebhooks(item model.Item) ([]InternalWebhook, error) {
	raw, err := json.Marshal(item.Data)
	if err != nil {
		return nil, err
	}

	switch {
	case has(item, schemaV2ItemKey):
		var m schema.ManifestV2
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		return fromSchemaV2(&m)
	case has(item, schemaV1ItemKey):
		// Duration is read separately: delivery uses Until, so a malformed
		// duration must not cost the registration.
		var m struct {
			PartnerIDs   []string
			Registration struct {
				// nolint:staticcheck // v1 must keep working
				webhook.RegistrationV1
				Duration json.RawMessage `json:"duration"`
			} `json:"wrp_event_stream_schema_v1"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		hook := fromSchemaV1(&schema.ManifestV1{PartnerIDs: m.PartnerIDs, Registration: m.Registration.RegistrationV1})
		hook.Webhook.Duration = lenientDuration(m.Registration.Duration)
		return []InternalWebhook{hook}, nil
	case has(item, legacyItemKey):
		var iw InternalWebhook
		if err := json.Unmarshal(raw, &iw); err != nil {
			return nil, err
		}
		return []InternalWebhook{iw}, nil
	default:
		return nil, errUnrecognizedItem
	}
}

func has(item model.Item, key string) bool {
	_, ok := item.Data[key]
	return ok
}

// lenientDuration reads a v1 duration (a Go duration string, or an integer in
// seconds) and returns zero when it is absent or malformed.
func lenientDuration(raw json.RawMessage) time.Duration {
	var d webhook.CustomDuration
	if err := json.Unmarshal(raw, &d); err != nil {
		return 0
	}

	return time.Duration(d)
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
