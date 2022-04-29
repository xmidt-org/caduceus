package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/bascule"
	bchecks "github.com/xmidt-org/bascule/basculechecks"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/bascule/key"
	"github.com/xmidt-org/webpa-common/v2/basculechecks"
	"github.com/xmidt-org/webpa-common/v2/basculemetrics"
	"github.com/xmidt-org/webpa-common/v2/logging"
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
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
	// Keys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory `json:"key"`

	// Leeway is used to set the amount of time buffer should be given to JWT
	// time values, such as nbf
	Leeway bascule.Leeway
}

func NewPrimaryHandler(l log.Logger, v *viper.Viper, registry xmetrics.Registry, sw *ServerHandler, webhookSvc ancla.Service, router *mux.Router, prevVersionSupport bool) (*mux.Router, error) {
	auth, err := authenticationMiddleware(v, l, registry)
	if err != nil {
		return nil, fmt.Errorf("unable to build authentication middleware: %v", err)
	}

	// if we want to support the previous API version, then include it in the
	// api base.
	urlPrefix := fmt.Sprintf("/%s/", apiBase)
	if prevVersionSupport {
		urlPrefix = fmt.Sprintf("/%s/", apiBaseDualVersion)
	}

	router.Handle(urlPrefix+"/notify", auth.Then(sw)).Methods("POST")

	return router, nil
}

// authenticationMiddleware configures the authorization requirements for requests to reach the main handler
func authenticationMiddleware(v *viper.Viper, logger log.Logger, registry xmetrics.Registry) (*alice.Chain, error) {
	if registry == nil {
		return nil, errors.New("nil registry")
	}

	basculeMeasures := basculemetrics.NewAuthValidationMeasures(registry)
	capabilityCheckMeasures := basculechecks.NewAuthCapabilityCheckMeasures(registry)
	listener := basculemetrics.NewMetricListener(basculeMeasures)

	basicAllowed := make(map[string]string)
	basicAuth := v.GetStringSlice("authHeader")
	for _, a := range basicAuth {
		decoded, err := base64.StdEncoding.DecodeString(a)
		if err != nil {
			level.Info(logger).Log(logging.MessageKey(), "failed to decode auth header", "authHeader", a, logging.ErrorKey(), err)
			continue
		}

		i := bytes.IndexByte(decoded, ':')
		level.Debug(logger).Log(logging.MessageKey(), "decoded string", "string", decoded, "i", i)
		if i > 0 {
			basicAllowed[string(decoded[:i])] = string(decoded[i+1:])
		}
	}
	level.Debug(logger).Log(logging.MessageKey(), "Created list of allowed basic auths", "allowed", basicAllowed, "config", basicAuth)

	options := []basculehttp.COption{
		basculehttp.WithCLogger(getLogger),
		basculehttp.WithCErrorResponseFunc(listener.OnErrorResponse),
		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc("/"+apiBase+"/", basculehttp.DefaultParseURLFunc)),
	}
	if len(basicAllowed) > 0 {
		options = append(options, basculehttp.WithTokenFactory("Basic", basculehttp.BasicTokenFactory(basicAllowed)))
	}

	var jwtVal JWTValidator
	v.UnmarshalKey("jwtValidator", &jwtVal)
	if jwtVal.Keys.URI != "" {
		resolver, err := jwtVal.Keys.NewResolver()
		if err != nil {
			return &alice.Chain{}, fmt.Errorf("failed to create resolver: %v", err)
		}

		options = append(options, basculehttp.WithTokenFactory("Bearer", basculehttp.BearerTokenFactory{
			DefaultKeyID: defaultKeyID,
			Resolver:     resolver,
			Parser:       bascule.DefaultJWTParser,
			Leeway:       jwtVal.Leeway,
		}))
	}

	authConstructor := basculehttp.NewConstructor(append([]basculehttp.COption{
		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc("/"+apiBase+"/", basculehttp.DefaultParseURLFunc)),
	}, options...)...)
	authConstructorLegacy := basculehttp.NewConstructor(append([]basculehttp.COption{
		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc("/api/"+prevAPIVersion+"/", basculehttp.DefaultParseURLFunc)),
		basculehttp.WithCErrorHTTPResponseFunc(basculehttp.LegacyOnErrorHTTPResponse),
	}, options...)...)

	bearerRules := bascule.Validators{
		bchecks.NonEmptyPrincipal(),
		bchecks.NonEmptyType(),
		bchecks.ValidType([]string{"jwt"}),
	}

	// only add capability check if the configuration is set
	var capabilityCheck CapabilityConfig
	v.UnmarshalKey("capabilityCheck", &capabilityCheck)
	if capabilityCheck.Type == "enforce" || capabilityCheck.Type == "monitor" {
		var endpoints []*regexp.Regexp
		c, err := basculechecks.NewEndpointRegexCheck(capabilityCheck.Prefix, capabilityCheck.AcceptAllMethod)
		if err != nil {
			return nil, fmt.Errorf("failed to create capability check: %v", err)
		}
		for _, e := range capabilityCheck.EndpointBuckets {
			r, err := regexp.Compile(e)
			if err != nil {
				level.Error(logger).Log(logging.MessageKey(), "failed to compile regular expression", "regex", e, logging.ErrorKey(), err.Error())
				continue
			}
			endpoints = append(endpoints, r)
		}
		m := basculechecks.MetricValidator{
			C:         basculechecks.CapabilitiesValidator{Checker: c},
			Measures:  capabilityCheckMeasures,
			Endpoints: endpoints,
		}
		bearerRules = append(bearerRules, m.CreateValidator(capabilityCheck.Type == "enforce"))
	}

	authEnforcer := basculehttp.NewEnforcer(
		basculehttp.WithELogger(getLogger),
		basculehttp.WithRules("Basic", bascule.Validators{
			bchecks.AllowAll(),
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
