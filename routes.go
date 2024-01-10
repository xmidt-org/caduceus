// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/xmidt-org/arrange/arrangehttp"
	"github.com/xmidt-org/arrange/arrangepprof"
	"github.com/xmidt-org/httpaux"
	"github.com/xmidt-org/touchstone/touchhttp"
	"go.uber.org/fx"
)

type RoutesIn struct {
	fx.In
	PrimaryMetrics   touchhttp.ServerInstrumenter `name:"servers.primary.metrics"`
	AlternateMetrics touchhttp.ServerInstrumenter `name:"servers.alternate.metrics"`
}

type RoutesOut struct {
	fx.Out
	Primary   arrangehttp.Option[http.Server] `group:"servers.primary.options"`
	Alternate arrangehttp.Option[http.Server] `group:"servers.alternate.options"`
}

// The name should be 'primary' or 'alternate'.
func provideCoreEndpoints() fx.Option {
	return fx.Provide(
		fx.Annotated{
			Name: "servers.primary.metrics",
			Target: touchhttp.ServerBundle{}.NewInstrumenter(
				touchhttp.ServerLabel, "primary",
			),
		},
		fx.Annotated{
			Name: "servers.alternate.metrics",
			Target: touchhttp.ServerBundle{}.NewInstrumenter(
				touchhttp.ServerLabel, "alternate",
			),
		},
		func(in RoutesIn) RoutesOut {
			return RoutesOut{
				Primary:   provideCoreOption("primary", in),
				Alternate: provideCoreOption("alternate", in),
			}
		},
	)
}

func provideCoreOption(server string, in RoutesIn) arrangehttp.Option[http.Server] {
	return arrangehttp.AsOption[http.Server](
		func(s *http.Server) {
			mux := chi.NewMux()
			if server == "primary" {
				s.Handler = in.PrimaryMetrics.Then(mux)
			} else {
				s.Handler = in.AlternateMetrics.Then(mux)
			}
		},
	)

}

func provideHealthCheck() fx.Option {
	return fx.Provide(
		fx.Annotated{
			Name: "servers.health.metrics",
			Target: touchhttp.ServerBundle{}.NewInstrumenter(
				touchhttp.ServerLabel, "health",
			),
		},
		fx.Annotate(
			func(metrics touchhttp.ServerInstrumenter, path HealthPath) arrangehttp.Option[http.Server] {
				return arrangehttp.AsOption[http.Server](
					func(s *http.Server) {
						mux := chi.NewMux()
						mux.Method("GET", string(path), httpaux.ConstantHandler{
							StatusCode: http.StatusOK,
						})
						s.Handler = metrics.Then(mux)
					},
				)
			},
			fx.ParamTags(`name:"servers.health.metrics"`),
			fx.ResultTags(`group:"servers.health.options"`),
		),
	)
}

func provideMetricEndpoint() fx.Option {
	return fx.Provide(
		fx.Annotate(
			func(metrics touchhttp.Handler, path MetricsPath) arrangehttp.Option[http.Server] {
				return arrangehttp.AsOption[http.Server](
					func(s *http.Server) {
						mux := chi.NewMux()
						mux.Method("GET", string(path), metrics)
						s.Handler = mux
					},
				)
			},
			fx.ResultTags(`group:"servers.metrics.options"`),
		),
	)
}

func providePprofEndpoint() fx.Option {
	return fx.Provide(
		fx.Annotate(
			func(pathPrefix PprofPathPrefix) arrangehttp.Option[http.Server] {
				return arrangehttp.AsOption[http.Server](
					func(s *http.Server) {
						s.Handler = arrangepprof.HTTP{
							PathPrefix: string(pathPrefix),
						}.New()
					},
				)
			},
			fx.ResultTags(`group:"servers.pprof.options"`),
		),
	)
}
