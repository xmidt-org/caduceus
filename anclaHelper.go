package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/golang-jwt/jwt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/argus/chrysom"
	"github.com/xmidt-org/bascule/acquire"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/webpa-common/v2/xmetrics"

	"go.uber.org/zap"
)

const (
	//Names
	WebhookListSizeGauge     = "webhook_list_size"
	ChrysomPollsTotalCounter = chrysom.PollCounter

	//Labels
	OutcomeLabel = "outcome"

	//Outcomes
	SuccessOutcome = "success"
	FailureOutcome = "failure"
)

type jwtAcquireParserType string

const (
	simpleType jwtAcquireParserType = "simple"
	rawType    jwtAcquireParserType = "raw"
)

var (
	errMissingExpClaim   = errors.New("missing exp claim in jwt")
	errUnexpectedCasting = errors.New("unexpected casting error")
)

type jwtAcquireParser struct {
	token      acquire.TokenParser
	expiration acquire.ParseExpiration
}

type HelperListenerConfig struct {
	Config chrysom.ListenerClientConfig

	// Logger for this package.
	// Gets passed to Argus config before initializing the client.
	// (Optional). Defaults to a no op logger.
	Logger *zap.Logger

	// Measures for instrumenting this package.
	// Gets passed to Argus config before initializing the client.
	Measures Measures
}

type Measures struct {
	WebhookListSizeGauge     metrics.Gauge
	ChrysomPollsTotalCounter *prometheus.CounterVec
}

// NewMeasures realizes desired metrics.
func NewMeasures(p xmetrics.Registry) *Measures {
	return &Measures{
		WebhookListSizeGauge:     p.NewGauge(WebhookListSizeGauge),
		ChrysomPollsTotalCounter: p.NewCounterVec(ChrysomPollsTotalCounter),
	}
}

func AnclaMetrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name: WebhookListSizeGauge,
			Type: xmetrics.GaugeType,
			Help: "Size of the current list of webhooks.",
		},
		{
			Name:       ChrysomPollsTotalCounter,
			Type:       xmetrics.CounterType,
			Help:       "Counter for the number of polls (and their success/failure outcomes) to fetch new items.",
			LabelNames: []string{OutcomeLabel},
		},
	}
}

type helperservice struct {
	argus  chrysom.PushReader
	logger *zap.Logger
	config ancla.Config
	now    func() time.Time
}

func NewHelperService(cfg ancla.Config, getLogger func(context.Context) *zap.Logger) (*helperservice, error) {
	if cfg.Logger == nil {
		cfg.Logger = sallust.Default()
	}
	prepArgusBasicClientConfig(&cfg)
	basic, err := chrysom.NewBasicClient(cfg.BasicClientConfig, getLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create chrysom basic client: %v", err)
	}
	svc := &helperservice{
		logger: cfg.Logger,
		argus:  basic,
		config: cfg,
		now:    time.Now,
	}
	return svc, nil
}

// StartListener builds the Argus listener client service from the given configuration.
// It allows adding watchers for the internal subscription state. Call the returned
// function when you are done watching for updates.
func (s *helperservice) StartHelperListener(cfg HelperListenerConfig, setLogger func(context.Context, *zap.Logger) context.Context, watches ...ancla.Watch) (func(), error) {
	if cfg.Logger == nil {
		cfg.Logger = sallust.Default()
	}
	prepArgusListenerClientConfig(&cfg, watches...)
	m := &chrysom.Measures{
		Polls: cfg.Measures.ChrysomPollsTotalCounter,
	}
	listener, err := chrysom.NewListenerClient(cfg.Config, setLogger, m, s.argus)
	if err != nil {
		return nil, fmt.Errorf("failed to create chrysom listener client: %v", err)
	}

	listener.Start(context.Background())
	return func() { listener.Stop(context.Background()) }, nil
}

func prepArgusBasicClientConfig(cfg *ancla.Config) error {
	p, err := newJWTAcquireParser(jwtAcquireParserType(cfg.JWTParserType))
	if err != nil {
		return err
	}
	cfg.BasicClientConfig.Auth.JWT.GetToken = p.token
	cfg.BasicClientConfig.Auth.JWT.GetExpiration = p.expiration
	return nil
}

func prepArgusListenerClientConfig(cfg *HelperListenerConfig, watches ...ancla.Watch) {
	logger := cfg.Logger
	watches = append(watches, webhookListSizeWatch(cfg.Measures.WebhookListSizeGauge))
	cfg.Config.Listener = chrysom.ListenerFunc(func(items chrysom.Items) {
		iws, err := ancla.ItemsToInternalWebhooks(items)
		if err != nil {
			logger.Error("Failed to convert items to webhooks", zap.Error(err))
			return
		}
		for _, watch := range watches {
			watch.Update(iws)
		}
	})
}

func webhookListSizeWatch(s metrics.Gauge) ancla.Watch {
	return ancla.WatchFunc(func(webhooks []ancla.InternalWebhook) {
		s.Set(float64(len(webhooks)))
	})
}

func newJWTAcquireParser(pType jwtAcquireParserType) (jwtAcquireParser, error) {
	if pType == "" {
		pType = simpleType
	}
	if pType != simpleType && pType != rawType {
		return jwtAcquireParser{}, errors.New("only 'simple' or 'raw' are supported as jwt acquire parser types")
	}
	// nil defaults are fine (bascule/acquire will use the simple
	// default parsers internally).
	var (
		tokenParser      acquire.TokenParser
		expirationParser acquire.ParseExpiration
	)
	if pType == rawType {
		tokenParser = rawTokenParser
		expirationParser = rawTokenExpirationParser
	}
	return jwtAcquireParser{expiration: expirationParser, token: tokenParser}, nil
}

func rawTokenParser(data []byte) (string, error) {
	return string(data), nil
}

func rawTokenExpirationParser(data []byte) (time.Time, error) {
	p := jwt.Parser{SkipClaimsValidation: true}
	token, _, err := p.ParseUnverified(string(data), jwt.MapClaims{})
	if err != nil {
		return time.Time{}, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return time.Time{}, errUnexpectedCasting
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
