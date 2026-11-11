// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/ancla/model"
	"github.com/xmidt-org/ancla/schema"
	webhook "github.com/xmidt-org/webhook-schema"
	"go.uber.org/zap"
)

// Produced by ancla v0.3.12: InternalWebhookToItem(...).Data and json.Marshal(InternalWebhook).
const (
	ancla03ItemData = `{"PartnerIDs":["comcast","partner2"],"Webhook":{"config":{"alt_urls":["http://alt.example.com/hook"],"content_type":"application/msgpack","secret":"s3cret","url":"http://receiver.example.com/hook"},"duration":300000000000,"events":["online","offline"],"failure_url":"http://failure.example.com/fail","matcher":{"device_id":["mac:112233445566.*"]},"registered_from_address":"10.0.0.1:1234","until":"2030-01-02T03:04:05Z"}}`
	ancla03Marshal  = `{"PartnerIDs":["comcast","partner2"],"Webhook":{"registered_from_address":"10.0.0.1:1234","config":{"url":"http://receiver.example.com/hook","content_type":"application/msgpack","secret":"s3cret","alt_urls":["http://alt.example.com/hook"]},"failure_url":"http://failure.example.com/fail","events":["online","offline"],"matcher":{"device_id":["mac:112233445566.*"]},"duration":300000000000,"until":"2030-01-02T03:04:05Z"}}`
)

var testUntil = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

func expectedWebhook() InternalWebhook {
	return InternalWebhook{
		PartnerIDs: []string{"comcast", "partner2"},
		Webhook: Webhook{
			Address: "10.0.0.1:1234",
			Config: DeliveryConfig{
				URL:             "http://receiver.example.com/hook",
				ContentType:     "application/msgpack",
				Secret:          "s3cret",
				AlternativeURLs: []string{"http://alt.example.com/hook"},
			},
			FailureURL: "http://failure.example.com/fail",
			Events:     []string{"online", "offline"},
			Matcher:    MetadataMatcherConfig{DeviceID: []string{"mac:112233445566.*"}},
			Duration:   5 * time.Minute,
			Until:      testUntil,
		},
	}
}

func itemFromJSON(t *testing.T, id, data string) model.Item {
	t.Helper()
	var d map[string]any
	require.NoError(t, json.Unmarshal([]byte(data), &d))
	return model.Item{ID: id, Data: d}
}

func TestItemToWebhooksAncla03(t *testing.T) {
	hooks, err := itemToWebhooks(itemFromJSON(t, "legacy", ancla03ItemData))
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	assert.Equal(t, expectedWebhook(), hooks[0])
}

// The cut-off notification embeds this structure, so its JSON must not change.
func TestInternalWebhookJSONMatchesAncla03(t *testing.T) {
	out, err := json.Marshal(expectedWebhook())
	require.NoError(t, err)
	assert.JSONEq(t, ancla03Marshal, string(out))
	assert.Equal(t, ancla03Marshal, string(out))
}

func TestItemToWebhooksSchemaV1(t *testing.T) {
	// The stored shape tr1d1um (ancla 0.5.5) writes, which ancla 0.3.12 could not decode.
	t.Run("stored shape", func(t *testing.T) {
		hooks, err := itemToWebhooks(itemFromJSON(t, "v1", `{"PartnerIDs":["comcast"],"wrp_event_stream_schema_v1":{"registered_from_address":"10.0.0.1:1234","config":{"url":"http://receiver.example.com/hook","content_type":"application/json"},"failure_url":"","events":["online"],"matcher":{"device_id":null},"duration":"5m0s","until":"2030-01-02T03:04:05Z"}}`))
		require.NoError(t, err)
		require.Len(t, hooks, 1)
		assert.Equal(t, []string{"comcast"}, hooks[0].PartnerIDs)
		assert.Equal(t, "http://receiver.example.com/hook", hooks[0].Webhook.Config.URL)
		assert.Equal(t, "application/json", hooks[0].Webhook.Config.ContentType)
		assert.Equal(t, []string{"online"}, hooks[0].Webhook.Events)
		assert.Equal(t, testUntil, hooks[0].Webhook.Until)
	})

	t.Run("round trip", testItemToWebhooksSchemaV1RoundTrip)
}

func testItemToWebhooksSchemaV1RoundTrip(t *testing.T) {
	manifest := &schema.ManifestV1{
		PartnerIDs: []string{"comcast", "partner2"},
		// nolint:staticcheck // v1 must keep working
		Registration: webhook.RegistrationV1{
			Address: "10.0.0.1:1234",
			// nolint:staticcheck // v1 must keep working
			Config: webhook.DeliveryConfig{
				ReceiverURL:     "http://receiver.example.com/hook",
				ContentType:     "application/msgpack",
				Secret:          "s3cret",
				AlternativeURLs: []string{"http://alt.example.com/hook"},
			},
			FailureURL: "http://failure.example.com/fail",
			Events:     []string{"online", "offline"},
			Matcher:    webhook.MetadataMatcherConfig{DeviceID: []string{"mac:112233445566.*"}},
			Duration:   webhook.CustomDuration(5 * time.Minute),
			Until:      testUntil,
		},
	}
	item, err := schema.SchemaToItem(time.Now, manifest)
	require.NoError(t, err)

	hooks, err := itemToWebhooks(item)
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	assert.Equal(t, expectedWebhook(), hooks[0])
}

func v2Item(t *testing.T, mutate func(*schema.ManifestV2)) model.Item {
	t.Helper()
	manifest := &schema.ManifestV2{
		PartnerIds: []string{"comcast"},
		Registration: webhook.RegistrationV2{
			CanonicalName: "my-registration",
			Address:       "10.0.0.1:1234",
			FailureURL:    "http://failure.example.com/fail",
			Expires:       testUntil,
			Webhooks: []webhook.Webhook{{
				Accept:       "application/msgpack",
				Secret:       "s3cret",
				ReceiverURLs: []string{"http://receiver.example.com/hook", "http://alt.example.com/hook"},
			}},
		},
	}
	if mutate != nil {
		mutate(manifest)
	}
	item, err := schema.SchemaToItem(time.Now, manifest)
	require.NoError(t, err)
	return item
}

func TestItemToWebhooksSchemaV2(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		hooks, err := itemToWebhooks(v2Item(t, nil))
		require.NoError(t, err)
		require.Len(t, hooks, 1)
		assert.Equal(t, InternalWebhook{
			PartnerIDs: []string{"comcast"},
			Webhook: Webhook{
				Address: "10.0.0.1:1234",
				Config: DeliveryConfig{
					URL:             "http://receiver.example.com/hook",
					ContentType:     "application/msgpack",
					Secret:          "s3cret",
					AlternativeURLs: []string{"http://alt.example.com/hook"},
				},
				FailureURL: "http://failure.example.com/fail",
				Events:     []string{".*"},
				Until:      testUntil,
			},
		}, hooks[0])
	})

	t.Run("matchers", func(t *testing.T) {
		hooks, err := itemToWebhooks(v2Item(t, func(m *schema.ManifestV2) {
			m.Registration.Matcher = []webhook.FieldRegex{
				{Field: "Destination", Regex: "event:device-status/.*/online"},
				{Field: "source", Regex: "mac:112233445566.*"},
			}
		}))
		require.NoError(t, err)
		require.Len(t, hooks, 1)
		assert.Equal(t, []string{"event:device-status/.*/online"}, hooks[0].Webhook.Events)
		assert.Equal(t, []string{"mac:112233445566.*"}, hooks[0].Webhook.Matcher.DeviceID)
	})

	t.Run("one registration per webhook sink", func(t *testing.T) {
		hooks, err := itemToWebhooks(v2Item(t, func(m *schema.ManifestV2) {
			m.Registration.Webhooks = append(m.Registration.Webhooks, webhook.Webhook{
				ReceiverURLs: []string{"http://second.example.com/hook"},
			})
		}))
		require.NoError(t, err)
		require.Len(t, hooks, 2)
		assert.Equal(t, "http://receiver.example.com/hook", hooks[0].Webhook.Config.URL)
		assert.Equal(t, "http://second.example.com/hook", hooks[1].Webhook.Config.URL)
	})

	t.Run("kafka sinks are ignored", func(t *testing.T) {
		hooks, err := itemToWebhooks(v2Item(t, func(m *schema.ManifestV2) {
			m.Registration.Kafkas = []webhook.Kafka{{BootstrapServers: []string{"kafka:9092"}}}
		}))
		require.NoError(t, err)
		assert.Len(t, hooks, 1)
	})

	for name, mutate := range map[string]func(*schema.ManifestV2){
		"hash": func(m *schema.ManifestV2) { m.Registration.Hash = webhook.FieldRegex{Field: "device_id", Regex: ".*"} },
		"unsupported field": func(m *schema.ManifestV2) {
			m.Registration.Matcher = []webhook.FieldRegex{{Field: "metadata/model", Regex: "x"}}
		},
		"repeated destination": func(m *schema.ManifestV2) {
			m.Registration.Matcher = []webhook.FieldRegex{{Field: "destination", Regex: "a"}, {Field: "destination", Regex: "b"}}
		},
		"repeated source": func(m *schema.ManifestV2) {
			m.Registration.Matcher = []webhook.FieldRegex{{Field: "source", Regex: "a"}, {Field: "Source", Regex: "b"}}
		},
		"no receiver urls": func(m *schema.ManifestV2) {
			m.Registration.Webhooks = []webhook.Webhook{{DNSSrvRecord: webhook.DNSSrvRecord{FQDNs: []string{"srv.example.com"}}}}
		},
		"no webhook sinks": func(m *schema.ManifestV2) {
			m.Registration.Webhooks = nil
			m.Registration.Kafkas = []webhook.Kafka{{BootstrapServers: []string{"kafka:9092"}}}
		},
	} {
		t.Run("rejected: "+name, func(t *testing.T) {
			_, err := itemToWebhooks(v2Item(t, mutate))
			assert.Error(t, err)
		})
	}
}

func TestItemToWebhooksUnrecognized(t *testing.T) {
	for name, data := range map[string]string{
		"unrelated": `{"something":"else"}`,
		"empty":     `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := itemToWebhooks(itemFromJSON(t, "x", data))
			assert.Error(t, err)
		})
	}

	_, err := itemToWebhooks(model.Item{ID: "nil"})
	assert.Error(t, err)
}

func TestItemToWebhooksMalformed(t *testing.T) {
	for name, data := range map[string]string{
		"v2 wrong type":     `{"wrp_event_stream_schema_v2":"x"}`,
		"v1 wrong type":     `{"wrp_event_stream_schema_v1":"x"}`,
		"legacy wrong type": `{"Webhook":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := itemToWebhooks(itemFromJSON(t, "x", data))
			require.Error(t, err)
		})
	}

	t.Run("data that cannot be encoded", func(t *testing.T) {
		_, err := itemToWebhooks(model.Item{ID: "x", Data: map[string]any{"bad": make(chan int)}})
		assert.Error(t, err)
	})
}

func TestItemsToWebhooksSkipsBadItems(t *testing.T) {
	items := []model.Item{
		itemFromJSON(t, "legacy", ancla03ItemData),
		itemFromJSON(t, "junk", `{"something":"else"}`),
		v2Item(t, nil),
		v2Item(t, func(m *schema.ManifestV2) { m.Registration.Hash.Field = "device_id" }),
	}

	hooks := itemsToWebhooks(items, zap.NewNop())
	require.Len(t, hooks, 2)
	assert.Equal(t, expectedWebhook(), hooks[0])
	assert.Equal(t, "http://receiver.example.com/hook", hooks[1].Webhook.Config.URL)
}
