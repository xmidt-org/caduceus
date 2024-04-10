// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package caduceus

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/xmidt-org/arrange/arrangehttp"
	"github.com/xmidt-org/arrange/arrangepprof"
	"github.com/xmidt-org/caduceus/internal/handler"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/httpaux"
	"github.com/xmidt-org/httpaux/recovery"
	"github.com/xmidt-org/touchstone/touchhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"

	"go.uber.org/fx"
)

const (
	apiVersion         = "v4"
	prevAPIVersion     = "v3"
	apiBase            = "api/" + apiVersion
	apiBaseDualVersion = "api/{version:" + apiVersion + "|" + prevAPIVersion + "}"
)

type RoutesIn struct {
	fx.In
	PrimaryMetrics         touchhttp.ServerInstrumenter `name:"servers.primary.metrics"`
	AlternateMetrics       touchhttp.ServerInstrumenter `name:"servers.alternate.metrics"`
	Handler                *handler.ServerHandler
	Tracing                candlelight.Tracing
	PreviousVersionSupport bool
}

type RoutesOut struct {
	fx.Out
	Primary   arrangehttp.Option[http.Server] `group:"servers.primary.options"`
	Alternate arrangehttp.Option[http.Server] `group:"servers.alternate.options"`
}

// The name should be 'primary' or 'alternate'.
func ProvideCoreEndpoints() fx.Option {
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
			urlPrefix := fmt.Sprintf("/%s", apiBase)
			if in.PreviousVersionSupport {
				urlPrefix = fmt.Sprintf("/%s", apiBaseDualVersion)
			}

			mux := chi.NewMux()

			options := []otelmux.Option{
				otelmux.WithTracerProvider(in.Tracing.TracerProvider()),
				otelmux.WithPropagators(in.Tracing.Propagator()),
			}

			// TODO: should probably customize things a bit
			mux.Use(recovery.Middleware(recovery.WithStatusCode(555)), otelmux.Middleware("server_primary", options...),
				candlelight.EchoFirstTraceNodeInfo(in.Tracing.Propagator(), true))

			mux.Method("POST", urlPrefix+"/notify", in.Handler)
			if server == "primary" {
				s.Handler = in.PrimaryMetrics.Then(mux)
			} else {
				s.Handler = in.AlternateMetrics.Then(mux)
			}
		},
	)

}

func ProvideHealthCheck() fx.Option {
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

func ProvideMetricEndpoint() fx.Option {
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

func ProvidePprofEndpoint() fx.Option {
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
