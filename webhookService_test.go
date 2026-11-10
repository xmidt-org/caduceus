// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/ancla/model"
	"github.com/xmidt-org/ancla/schema"
	"github.com/xmidt-org/bascule/acquire"
	"go.uber.org/zap"
)

type recordingWatcher struct {
	mutex sync.Mutex
	lists [][]InternalWebhook
}

func (rw *recordingWatcher) Update(list []InternalWebhook) {
	rw.mutex.Lock()
	defer rw.mutex.Unlock()
	rw.lists = append(rw.lists, list)
}

func (rw *recordingWatcher) last() []InternalWebhook {
	rw.mutex.Lock()
	defer rw.mutex.Unlock()
	if len(rw.lists) == 0 {
		return nil
	}
	return rw.lists[len(rw.lists)-1]
}

// Argus holds items written by ancla 0.3.x and 0.5.x side by side during a rollout.
func TestStartWebhookListenerMixedFormats(t *testing.T) {
	legacy := itemFromJSON(t, "legacy", ancla03ItemData)
	v2 := v2Item(t, nil)

	var (
		mutex      sync.Mutex
		authHeader string
		path       string
	)
	argus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		authHeader, path = r.Header.Get("Authorization"), r.URL.Path
		mutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]model.Item{legacy, v2})
	}))
	t.Cleanup(argus.Close)

	measures := WebhookMeasures{
		ListSize:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "list_size"}),
		PollsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "polls"}, []string{outcomeLabel}),
	}
	logger := zap.NewNop()
	watcher := &recordingWatcher{}

	stop, err := startWebhookListener(
		WebhookConfig{BasicClientConfig: ArgusClientConfig{
			Address: argus.URL,
			Bucket:  "webhooks",
			Auth:    ArgusAuth{Basic: "Basic dXNlcjpwYXNz"},
		}},
		ListenerConfig{Config: ListenerClientConfig{PullInterval: 10 * time.Millisecond}},
		measures, argus.Client(), logger,
		func(context.Context) *zap.Logger { return logger },
		func(ctx context.Context, _ *zap.Logger) context.Context { return ctx },
		watcher,
	)
	require.NoError(t, err)
	t.Cleanup(stop)

	require.Eventually(t, func() bool { return len(watcher.last()) == 2 }, 5*time.Second, 10*time.Millisecond)

	list := watcher.last()
	assert.Equal(t, expectedWebhook(), list[0])
	assert.Equal(t, "http://receiver.example.com/hook", list[1].Webhook.Config.URL)
	assert.Equal(t, schemaV2Until(t, v2), list[1].Webhook.Until)

	mutex.Lock()
	defer mutex.Unlock()
	assert.Equal(t, "Basic dXNlcjpwYXNz", authHeader)
	assert.Equal(t, "/api/v1/store/webhooks", path)
}

func schemaV2Until(t *testing.T, item model.Item) time.Time {
	t.Helper()
	m, err := schema.ItemToSchema(item)
	require.NoError(t, err)
	return m.GetUntil()
}

func TestStartWebhookListenerConfigErrors(t *testing.T) {
	logger := zap.NewNop()
	getLogger := func(context.Context) *zap.Logger { return logger }
	setLogger := func(ctx context.Context, _ *zap.Logger) context.Context { return ctx }

	for name, cfg := range map[string]WebhookConfig{
		"no address": {BasicClientConfig: ArgusClientConfig{Bucket: "webhooks"}},
		"no bucket":  {BasicClientConfig: ArgusClientConfig{Address: "http://argus:6600"}},
		"bad parser": {
			JWTParserType: "other",
			BasicClientConfig: ArgusClientConfig{
				Address: "http://argus:6600", Bucket: "webhooks",
				Auth: ArgusAuth{JWT: acquire.RemoteBearerTokenAcquirerOptions{
					AuthURL: "http://auth.example.com", Buffer: time.Minute, Timeout: time.Minute,
				}},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			stop, err := startWebhookListener(cfg, ListenerConfig{}, WebhookMeasures{}, http.DefaultClient,
				logger, getLogger, setLogger)
			assert.Error(t, err)
			assert.Nil(t, stop)
		})
	}
}
