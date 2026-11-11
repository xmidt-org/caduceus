// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestItemToWebhooksDurationEncodings(t *testing.T) {
	const v1 = `{"PartnerIDs":["comcast"],"wrp_event_stream_schema_v1":{"config":{"url":"http://receiver.example.com/hook","content_type":"application/json"},"events":["online"],"duration":%s,"until":"2030-01-02T03:04:05Z"}}`
	const legacy = `{"PartnerIDs":["comcast"],"Webhook":{"config":{"url":"http://receiver.example.com/hook","content_type":"application/json"},"events":["online"],"duration":%s,"until":"2030-01-02T03:04:05Z"}}`

	for name, test := range map[string]struct {
		format   string
		duration string
		expected time.Duration
	}{
		"v1 go string":           {v1, `"5m0s"`, 5 * time.Minute},
		"v1 short go string":     {v1, `"5m"`, 5 * time.Minute},
		"v1 integer is nano":     {v1, `300000000000`, 5 * time.Minute},
		"v1 absent":              {v1, `null`, 0},
		"legacy integer is nano": {legacy, `300000000000`, 5 * time.Minute},
		"legacy zero":            {legacy, `0`, 0},
	} {
		t.Run(name, func(t *testing.T) {
			item := itemFromJSON(t, "x", fmt.Sprintf(test.format, test.duration))
			hooks, err := itemToWebhooks(item)
			require.NoError(t, err)
			require.Len(t, hooks, 1)
			assert.Equal(t, test.expected, hooks[0].Webhook.Duration)
			assert.Equal(t, testUntil, hooks[0].Webhook.Until)
			assert.Equal(t, "http://receiver.example.com/hook", hooks[0].Webhook.Config.URL)
		})
	}
}

func TestItemToWebhooksMalformedDuration(t *testing.T) {
	item := itemFromJSON(t, "x", `{"PartnerIDs":["comcast"],"wrp_event_stream_schema_v1":{"duration":"soon"}}`)
	_, err := itemToWebhooks(item)
	assert.Error(t, err)
}
