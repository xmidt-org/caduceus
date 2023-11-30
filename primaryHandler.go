// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"emperror.dev/emperror"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/basculechecks"
	"github.com/xmidt-org/bascule/basculehelper"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/clortho"
	"github.com/xmidt-org/clortho/clorthometrics"
	"github.com/xmidt-org/clortho/clorthozap"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
	"go.uber.org/zap"
)

const (
	apiVersion         = "v4"
	prevAPIVersion     = "v3"
	apiBase            = "api/" + apiVersion
	apiBaseDualVersion = "api/{version:" + apiVersion + "|" + prevAPIVersion + "}"
)

type CapabilityConfig struct {
	Type            string
	Prefix          string
	AcceptAllMethod string
	EndpointBuckets []string
}

// JWTValidator provides a convenient way to define jwt validator through config files
type JWTValidator struct {
	// Config is used to create the clortho Resolver & Refresher for JWT verification keys
	Config clortho.Config `json:"config"`

	// Leeway is used to set the amount of time buffer should be given to JWT
	// time values, such as nbf
	Leeway bascule.Leeway
}

func NewPrimaryHandler(l *zap.Logger, v *viper.Viper, registry xmetrics.Registry, sw *ServerHandler, webhookSvc ancla.Service, router *mux.Router, prevVersionSupport bool) (*mux.Router, error) {
	auth, err := authenticationMiddleware(v, l, registry)
	if err != nil {
		// nolint:errorlint
		return nil, fmt.Errorf("unable to build authentication middleware: %v", err)
	}

	// if we want to support the previous API version, then include it in the
	// api base.
	urlPrefix := fmt.Sprintf("/%s", apiBase)
	if prevVersionSupport {
		urlPrefix = fmt.Sprintf("/%s", apiBaseDualVersion)
	}

	router.Handle(urlPrefix+"/notify", auth.Then(sw)).Methods("POST")

	return router, nil
}

// authenticationMiddleware configures the authorization requirements for requests to reach the main handler
func authenticationMiddleware(v *viper.Viper, logger *zap.Logger, registry xmetrics.Registry) (*alice.Chain, error) {
	if registry == nil {
		return nil, errors.New("nil registry")
	}

	basculeMeasures := basculehelper.NewAuthValidationMeasures(registry)
	capabilityCheckMeasures := basculehelper.NewAuthCapabilityCheckMeasures(registry)
	listener := basculehelper.NewMetricListener(basculeMeasures)

	basicAllowed := make(map[string]string)
	basicAuth := v.GetStringSlice("authHeader")
	for _, a := range basicAuth {
		decoded, err := base64.StdEncoding.DecodeString(a)
		if err != nil {
			logger.Info("failed to decode auth header", zap.String("authHeader", a), zap.Error(err))
			continue
		}

		i := bytes.IndexByte(decoded, ':')
		logger.Debug("decoded string", zap.ByteString("string", decoded), zap.Int("i", i))
		if i > 0 {
			basicAllowed[string(decoded[:i])] = string(decoded[i+1:])
		}
	}
	logger.Debug("Created list of allowed basic auths", zap.Any("allowed", basicAllowed), zap.Any("config", basicAuth))

	options := []basculehttp.COption{
		basculehttp.WithCLogger(getLogger),
		basculehttp.WithCErrorResponseFunc(listener.OnErrorResponse),
	}
	if len(basicAllowed) > 0 {
		options = append(options, basculehttp.WithTokenFactory("Basic", basculehttp.BasicTokenFactory(basicAllowed)))
	}

	var jwtVal JWTValidator
	// Get jwt configuration, including clortho's configuration
	v.UnmarshalKey("jwtValidator", &jwtVal)
	kr := clortho.NewKeyRing()

	// Instantiate a fetcher for refresher and resolver to share
	f, err := clortho.NewFetcher()
	if err != nil {
		return &alice.Chain{}, emperror.With(err, "failed to create clortho fetcher")
	}

	ref, err := clortho.NewRefresher(
		clortho.WithConfig(jwtVal.Config),
		clortho.WithFetcher(f),
	)
	if err != nil {
		return &alice.Chain{}, emperror.With(err, "failed to create clortho refresher")
	}

	resolver, err := clortho.NewResolver(
		clortho.WithConfig(jwtVal.Config),
		clortho.WithKeyRing(kr),
		clortho.WithFetcher(f),
	)
	if err != nil {
		return &alice.Chain{}, emperror.With(err, "failed to create clortho resolver")
	}

	promReg, ok := registry.(prometheus.Registerer)
	if !ok {
		return &alice.Chain{}, errors.New("failed to get prometheus registerer")
	}

	var (
		tsConfig touchstone.Config
		zConfig  sallust.Config
	)
	// Get touchstone & zap configurations
	v.UnmarshalKey("touchstone", &tsConfig)
	v.UnmarshalKey("zap", &zConfig)
	zlogger := zap.Must(zConfig.Build())
	tf := touchstone.NewFactory(tsConfig, zlogger, promReg)
	// Instantiate a metric listener for refresher and resolver to share
	cml, err := clorthometrics.NewListener(clorthometrics.WithFactory(tf))
	if err != nil {
		return &alice.Chain{}, emperror.With(err, "failed to create clortho metrics listener")
	}

	// Instantiate a logging listener for refresher and resolver to share
	czl, err := clorthozap.NewListener(
		clorthozap.WithLogger(zlogger),
	)
	if err != nil {
		return &alice.Chain{}, emperror.With(err, "failed to create clortho zap logger listener")
	}

	resolver.AddListener(cml)
	resolver.AddListener(czl)
	ref.AddListener(cml)
	ref.AddListener(czl)
	ref.AddListener(kr)
	// context.Background() is for the unused `context.Context` argument in refresher.Start
	ref.Start(context.Background())
	// Shutdown refresher's goroutines when SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	go func() {
		<-sigs
		// context.Background() is for the unused `context.Context` argument in refresher.Stop
		ref.Stop(context.Background())
	}()

	options = append(options, basculehttp.WithTokenFactory("Bearer", basculehttp.BearerTokenFactory{
		DefaultKeyID: "current",
		Resolver:     resolver,
		Parser:       bascule.DefaultJWTParser,
		Leeway:       jwtVal.Leeway,
	}))
	authConstructor := basculehttp.NewConstructor(append([]basculehttp.COption{
		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc("/"+apiBase+"/", basculehttp.DefaultParseURLFunc)),
	}, options...)...)
	authConstructorLegacy := basculehttp.NewConstructor(append([]basculehttp.COption{
		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc("/api/"+prevAPIVersion+"/", basculehttp.DefaultParseURLFunc)),
		basculehttp.WithCErrorHTTPResponseFunc(basculehttp.LegacyOnErrorHTTPResponse),
	}, options...)...)
	bearerRules := bascule.Validators{
		basculechecks.NonEmptyPrincipal(),
		basculechecks.NonEmptyType(),
		basculechecks.ValidType([]string{"jwt"}),
	}

	// only add capability check if the configuration is set
	var capabilityCheck CapabilityConfig
	v.UnmarshalKey("capabilityCheck", &capabilityCheck)
	if capabilityCheck.Type == "enforce" || capabilityCheck.Type == "monitor" {
		var endpoints []*regexp.Regexp
		c, err := basculehelper.NewEndpointRegexCheck(capabilityCheck.Prefix, capabilityCheck.AcceptAllMethod)
		if err != nil {

			// nolint:errorlint
			return nil, fmt.Errorf("failed to create capability check: %v", err)
		}
		for _, e := range capabilityCheck.EndpointBuckets {
			r, err := regexp.Compile(e)
			if err != nil {
				logger.Error("failed to compile regular expression", zap.Any("regex", e), zap.Error(err))
				continue
			}
			endpoints = append(endpoints, r)
		}
		m := basculehelper.MetricValidator{
			C:         basculehelper.CapabilitiesValidator{Checker: c},
			Measures:  capabilityCheckMeasures,
			Endpoints: endpoints,
		}
		bearerRules = append(bearerRules, m.CreateValidator(capabilityCheck.Type == "enforce"))
	}

	authEnforcer := basculehttp.NewEnforcer(
		basculehttp.WithELogger(getLogger),
		basculehttp.WithRules("Basic", bascule.Validators{
			basculechecks.AllowAll(),
		}),
		basculehttp.WithRules("Bearer", bearerRules),
		basculehttp.WithEErrorResponseFunc(listener.OnErrorResponse),
	)

	authChain := alice.New(setLogger(logger), authConstructor, authEnforcer, basculehttp.NewListenerDecorator(listener))
	authChainLegacy := alice.New(setLogger(logger), authConstructorLegacy, authEnforcer, basculehttp.NewListenerDecorator(listener))

	versionCompatibleAuth := alice.New(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(r http.ResponseWriter, req *http.Request) {
			vars := mux.Vars(req)
			if vars != nil {
				if vars["version"] == prevAPIVersion {
					authChainLegacy.Then(next).ServeHTTP(r, req)
					return
				}
			}
			authChain.Then(next).ServeHTTP(r, req)
		})
	})
	return &versionCompatibleAuth, nil
}
