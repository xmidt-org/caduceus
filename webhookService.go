// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
	"github.com/xmidt-org/ancla/auth"
	"github.com/xmidt-org/ancla/chrysom"
	"github.com/xmidt-org/bascule/acquire"
	"go.uber.org/zap"
)

// Metric names are unchanged from ancla 0.3.x so existing dashboards keep working.
const (
	webhookListSizeGaugeName     = "webhook_list_size"
	webhookListSizeGaugeHelp     = "Size of the current list of webhooks."
	chrysomPollsTotalCounterName = "chrysom_polls_total"
	chrysomPollsTotalCounterHelp = "Counter for the number of polls (and their success/failure outcomes) to fetch new items."
	outcomeLabel                 = chrysom.OutcomeLabel
)

// WebhookConfig keeps the configuration layout ancla 0.3.x used.
type WebhookConfig struct {
	// JWTParserType is 'simple' (default) or 'raw'.
	JWTParserType     string
	BasicClientConfig ArgusClientConfig
}

type ArgusClientConfig struct {
	Address string
	Bucket  string
	Auth    ArgusAuth
}

type ArgusAuth struct {
	JWT   acquire.RemoteBearerTokenAcquirerOptions
	Basic string
}

type ListenerConfig struct {
	Config ListenerClientConfig
}

type ListenerClientConfig struct {
	// PullInterval is how often the webhook list is refreshed.  Defaults to 5s.
	PullInterval time.Duration
}

type WebhookMeasures struct {
	ListSize   prometheus.Gauge
	PollsTotal *prometheus.CounterVec
}

// WebhookWatcher is told the full list of webhooks each time it is refreshed.
type WebhookWatcher interface {
	Update([]InternalWebhook)
}

var errEmptyArgusAddress = errors.New("webhook.BasicClientConfig.address must not be empty")

// startWebhookListener polls Argus and passes the decoded webhooks to the
// watchers.  The returned function stops the polling.
func startWebhookListener(
	cfg WebhookConfig,
	listenerCfg ListenerConfig,
	measures WebhookMeasures,
	httpClient *http.Client,
	logger *zap.Logger,
	getLogger func(context.Context) *zap.Logger,
	setLogger func(context.Context, *zap.Logger) context.Context,
	watchers ...WebhookWatcher,
) (func(), error) {
	if cfg.BasicClientConfig.Address == "" {
		return nil, errEmptyArgusAddress
	}

	decorator, err := argusAuthDecorator(cfg.JWTParserType, cfg.BasicClientConfig.Auth)
	if err != nil {
		return nil, err
	}

	clientOut, err := chrysom.ProvideBasicClient(chrysom.ProvideBasicClientIn{
		Options: chrysom.ClientOptions{
			chrysom.StoreBaseURL(cfg.BasicClientConfig.Address),
			chrysom.Bucket(cfg.BasicClientConfig.Bucket),
			chrysom.HTTPClient(httpClient),
			chrysom.GetClientLogger(getLogger),
			chrysom.Auth(decorator),
		},
	})
	if err != nil {
		return nil, err
	}

	update := chrysom.ListenerFunc(func(_ context.Context, items chrysom.Items) error {
		hooks := itemsToWebhooks(items, logger)
		measures.ListSize.Set(float64(len(hooks)))
		for _, w := range watchers {
			w.Update(hooks)
		}
		return nil
	})

	listener, err := chrysom.NewListenerClient(measures.PollsTotal,
		clientOut.Reader,
		chrysom.Listener(update),
		chrysom.PullInterval(listenerCfg.Config.PullInterval),
		chrysom.GetListenerLogger(func(context.Context) *zap.Logger { return logger }),
		chrysom.SetListenerLogger(setLogger),
	)
	if err != nil {
		return nil, err
	}

	if err := listener.Start(context.Background()); err != nil {
		return nil, err
	}

	return func() { listener.Stop(context.Background()) }, nil
}

func argusAuthDecorator(parserType string, a ArgusAuth) (auth.Decorator, error) {
	acquirer, err := argusAcquirer(parserType, a)
	if err != nil {
		return nil, err
	}

	return auth.DecoratorFunc(func(_ context.Context, r *http.Request) error {
		return acquire.AddAuth(r, acquirer)
	}), nil
}

func argusAcquirer(parserType string, a ArgusAuth) (acquire.Acquirer, error) {
	if len(a.JWT.AuthURL) > 0 && a.JWT.Buffer != 0 && a.JWT.Timeout != 0 {
		parser, err := newJWTAcquireParser(parserType)
		if err != nil {
			return nil, err
		}
		opts := a.JWT
		opts.GetToken, opts.GetExpiration = parser.token, parser.expiration
		return acquire.NewRemoteBearerTokenAcquirer(opts)
	}

	if len(a.Basic) > 0 {
		return acquire.NewFixedAuthAcquirer(a.Basic)
	}

	return &acquire.DefaultAcquirer{}, nil
}

const (
	simpleJWTParser = "simple"
	rawJWTParser    = "raw"
)

var errMissingExpClaim = errors.New("missing exp claim in jwt")

type jwtAcquireParser struct {
	token      acquire.TokenParser
	expiration acquire.ParseExpiration
}

// newJWTAcquireParser returns nil functions for 'simple' so the acquirer uses its defaults.
func newJWTAcquireParser(parserType string) (jwtAcquireParser, error) {
	switch parserType {
	case "", simpleJWTParser:
		return jwtAcquireParser{}, nil
	case rawJWTParser:
		return jwtAcquireParser{token: rawTokenParser, expiration: rawTokenExpirationParser}, nil
	default:
		return jwtAcquireParser{}, errors.New("only 'simple' or 'raw' are supported as jwt acquire parser types")
	}
}

func rawTokenParser(data []byte) (string, error) {
	return string(data), nil
}

func rawTokenExpirationParser(data []byte) (time.Time, error) {
	p := jwt.Parser{SkipClaimsValidation: true}
	claims := jwt.MapClaims{}
	if _, _, err := p.ParseUnverified(string(data), claims); err != nil {
		return time.Time{}, err
	}

	expVal, ok := claims["exp"]
	if !ok {
		return time.Time{}, errMissingExpClaim
	}

	exp, err := cast.ToInt64E(expVal)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(exp, 0), nil
}
