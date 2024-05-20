// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/caduceus/internal/metrics"
	"github.com/xmidt-org/candlelight"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/fx"
)

var (
	errNilHistogram = errors.New("histogram cannot be nil")
)

type HttpClientIn struct {
	fx.In
	Timeouts HttpClientTimeout
	Tracing  candlelight.Tracing
}

// httpClientTimeout contains timeouts for an HTTP client and its requests.
type HttpClientTimeout struct {
	// ClientTimeout is HTTP Client Timeout.
	ClientTimeout time.Duration

	// NetDialerTimeout is the net dialer timeout
	NetDialerTimeout time.Duration
}
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
	return DoerFunc(func(req *http.Request) (*http.Response, error) {
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

func Provide() fx.Option {
	return fx.Provide(
		func(in HttpClientIn) *http.Client {
			if in.Timeouts.ClientTimeout == 0 {
				in.Timeouts.ClientTimeout = time.Second * 50

			}

			if in.Timeouts.NetDialerTimeout == 0 {
				in.Timeouts.NetDialerTimeout = time.Second * 5
			}

			var transport http.RoundTripper = &http.Transport{
				Dial: (&net.Dialer{
					Timeout: in.Timeouts.NetDialerTimeout,
				}).Dial,
			}

			transport = otelhttp.NewTransport(transport,
				otelhttp.WithPropagators(in.Tracing.Propagator()),
				otelhttp.WithTracerProvider(in.Tracing.TracerProvider()),
			)

			return &http.Client{
				Timeout:   in.Timeouts.ClientTimeout,
				Transport: transport,
			}
		},
	)
}
