package main

import (
	"fmt"

	"github.com/SermoDigital/jose/jwt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/provider"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/webpa-common/v2/secure"
	"github.com/xmidt-org/webpa-common/v2/secure/handler"
	"github.com/xmidt-org/webpa-common/v2/secure/key"
)

const (
	baseURI = "api"
	version = "v3"
)

type JWTValidator struct {
	// JWTKeys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory

	// Custom is an optional configuration section that defines
	// custom rules for validation over and above the standard RFC rules.
	Custom secure.JWTValidatorFactory
}

func NewPrimaryHandler(l log.Logger, v *viper.Viper, sw *ServerHandler, webhookSvc ancla.Service, metricsRegistry provider.Provider, router *mux.Router) (*mux.Router, error) {

	validator, err := getValidator(v)
	if err != nil {
		return nil, err
	}

	authHandler := handler.AuthorizationHandler{
		HeaderName:          "Authorization",
		ForbiddenStatusCode: 403,
		Validator:           validator,
		Logger:              l,
	}

	authorizationDecorator := alice.New(setLogger(l), authHandler.Decorate)

	return configServerRouter(router, authorizationDecorator, sw, webhookSvc, metricsRegistry), nil
}

func configServerRouter(router *mux.Router, primaryHandler alice.Chain, serverWrapper *ServerHandler, webhookSvc ancla.Service, metricsRegistry provider.Provider) *mux.Router {

	router.Handle("/"+fmt.Sprintf("%s/%s", baseURI, version)+"/notify", primaryHandler.Then(serverWrapper)).Methods("POST")

	addWebhookHandler := ancla.NewAddWebhookHandler(webhookSvc, ancla.HandlerConfig{
		MetricsProvider: metricsRegistry,
	})
	// register webhook end points
	router.Handle("/hook", primaryHandler.Then(addWebhookHandler)).Methods("POST")

	return router
}

func getValidator(v *viper.Viper) (validator secure.Validator, err error) {
	var jwtVals []JWTValidator

	v.UnmarshalKey("jwtValidators", &jwtVals)

	// if a JWTKeys section was supplied, configure a JWS validator
	// and append it to the chain of validators
	validators := make(secure.Validators, 0, len(jwtVals))

	for _, validatorDescriptor := range jwtVals {
		var keyResolver key.Resolver
		keyResolver, err = validatorDescriptor.Keys.NewResolver()
		if err != nil {
			validator = validators
			return
		}

		validators = append(
			validators,
			secure.JWSValidator{
				DefaultKeyId:  DEFAULT_KEY_ID,
				Resolver:      keyResolver,
				JWTValidators: []*jwt.Validator{validatorDescriptor.Custom.New()},
			},
		)
	}

	// TODO: This should really be part of the unmarshalled validators somehow
	basicAuth := v.GetStringSlice("authHeader")
	for _, authValue := range basicAuth {
		validators = append(
			validators,
			secure.ExactMatchValidator(authValue),
		)
	}

	validator = validators

	return
}
