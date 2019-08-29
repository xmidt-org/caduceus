package main

import (
	"fmt"
	"net/http"

	"github.com/xmidt-org/webpa-common/secure"
	"github.com/xmidt-org/webpa-common/secure/handler"
	"github.com/xmidt-org/webpa-common/secure/key"
	"github.com/xmidt-org/webpa-common/webhook"
	"github.com/SermoDigital/jose/jwt"
	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
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

func NewPrimaryHandler(l log.Logger, v *viper.Viper, sw *ServerHandler, reg *webhook.Registry) (*mux.Router, error) {
	var (
		router = mux.NewRouter()
	)

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

	authorizationDecorator := alice.New(authHandler.Decorate)

	return configServerRouter(router, authorizationDecorator, sw, reg), nil
}

func configServerRouter(router *mux.Router, primaryHandler alice.Chain, serverWrapper *ServerHandler, webhookRegistry *webhook.Registry) *mux.Router {
	var singleContentType = func(r *http.Request, _ *mux.RouteMatch) bool {
		return len(r.Header["Content-Type"]) == 1 //require single specification for Content-Type Header
	}

	router.Handle("/"+fmt.Sprintf("%s/%s", baseURI, version)+"/notify", primaryHandler.Then(serverWrapper)).Methods("POST").HeadersRegexp("Content-Type", "application/msgpack").MatcherFunc(singleContentType)

	// register webhook end points
	router.Handle("/hook", primaryHandler.ThenFunc(webhookRegistry.UpdateRegistry)).Methods("POST")
	router.Handle("/hooks", primaryHandler.ThenFunc(webhookRegistry.GetRegistry)).Methods("GET")

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
