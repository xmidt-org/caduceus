// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"
)

// The webhook metric names predate the ancla upgrade and dashboards depend on them.
func TestMetricsWebhookMeasures(t *testing.T) {
	cfg := touchstone.Config{DefaultNamespace: "n", DefaultSubsystem: "s"}
	gatherer, registerer, err := touchstone.New(cfg)
	require.NoError(t, err)

	_, _, _, measures, err := Metrics(touchstone.NewFactory(cfg, sallust.Default(), registerer))
	require.NoError(t, err)
	require.NotNil(t, measures.ListSize)
	require.NotNil(t, measures.PollsTotal)

	measures.ListSize.Set(3)
	measures.PollsTotal.With(prometheus.Labels{outcomeLabel: "success"}).Add(1)

	families, err := gatherer.Gather()
	require.NoError(t, err)

	names := map[string]bool{}
	for _, family := range families {
		names[family.GetName()] = true
	}
	assert.True(t, names["n_s_webhook_list_size"], "webhook_list_size missing: %v", keys(names))
	assert.True(t, names["n_s_chrysom_polls_total"], "chrysom_polls_total missing: %v", keys(names))
}

func keys(m map[string]bool) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return strings.Join(out, ",")
}
