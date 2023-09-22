// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"github.com/xmidt-org/ancla"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

func NewHelperMeasures(p xmetrics.Registry) ancla.Measures {
	return ancla.Measures{
		ChrysomPollsTotalCounterName: p.NewCounterVec(ancla.ChrysomPollsTotalCounterName),
		WebhookListSizeGaugeName:     p.NewPrometheusGauge(ancla.WebhookListSizeGaugeName),
	}
}

func AnclaHelperMetrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name:       ancla.ChrysomPollsTotalCounterName,
			Type:       xmetrics.CounterType,
			Help:       "Counter for the number of polls (and their success/failure outcomes) to fetch new items.",
			LabelNames: []string{ancla.OutcomeLabel},
		},
	}
}
