// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/caduceus/internal/metrics"
)

var (
	errNilHistogram = errors.New("histogram cannot be nil")
)

type metricWrapper struct {
	now          func() time.Time
	queryLatency prometheus.ObserverVec
	id           string
}

func NewMetricWrapper(now func() time.Time, queryLatency prometheus.ObserverVec, id string) (*metricWrapper, error) {
	if now == nil {
		now = time.Now
	}
	if queryLatency == nil {
		return nil, errNilHistogram
	}
	return &metricWrapper{
		now:          now,
		queryLatency: queryLatency,
		id:           id,
	}, nil
}

func (m *metricWrapper) RoundTripper(next Client) Client {
	return doerFunc(func(req *http.Request) (*http.Response, error) {
		startTime := m.now()
		resp, err := next.Do(req)
		endTime := m.now()
		code := metrics.GenericDoReason
		reason := metrics.NoErrReason
		if err != nil {
			reason = metrics.GetDoErrReason(err)
			if resp != nil {
				code = strconv.Itoa(resp.StatusCode)
			}
		} else {
			code = strconv.Itoa(resp.StatusCode)
		}

		// find time difference, add to metric
		m.queryLatency.With(prometheus.Labels{"id": m.id, metrics.UrlLabel: req.URL.String(), metrics.CodeLabel: code, metrics.ReasonLabel: reason}).Observe(endTime.Sub(startTime).Seconds())
		return resp, err
	})
}
