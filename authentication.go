// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/go-kit/kit/metrics"
	"github.com/spf13/cast"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/basculechecks"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

const (
	reasonMissingValues  = "auth_is_missing_values"
	reasonUndeterminedID = "undetermined_partner_ID"
)

func authCapabilitiesMetrics() []xmetrics.Metric {
	return []xmetrics.Metric{{
		Name:       "auth_capability_check",
		Type:       xmetrics.CounterType,
		Help:       "Counter for the capability checker, providing outcome information by client, partner, and endpoint",
		LabelNames: []string{"outcome", "reason", "clientid", "partnerid", "endpoint"},
	}}
}

func authValidationMetrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name:       "auth_validation",
			Type:       xmetrics.CounterType,
			Help:       "Counter for success and failure reason results through bascule",
			LabelNames: []string{"outcome"},
		},
		{
			Name:    "auth_from_nbf_seconds",
			Type:    xmetrics.HistogramType,
			Help:    "Difference (in seconds) between time of JWT validation and nbf (including leeway)",
			Buckets: []float64{-61, -11, -2, -1, 0, 9, 60},
		},
		{
			Name:    "auth_from_exp_seconds",
			Type:    xmetrics.HistogramType,
			Help:    "Difference (in seconds) between time of JWT validation and exp (including leeway)",
			Buckets: []float64{-61, -11, -2, -1, 0, 9, 60},
		},
	}
}

type authMetricListener struct {
	counter metrics.Counter
}

func (listener authMetricListener) OnErrorResponse(reason basculehttp.ErrorResponseReason, _ error) {
	listener.counter.With("outcome", reason.String()).Add(1)
}

func (listener authMetricListener) OnAuthenticated(auth bascule.Authentication) {
	if auth.Token != nil {
		listener.counter.With("outcome", "Accepted").Add(1)
	}
}

type capabilityEndpointCheck struct {
	prefix    *regexp.Regexp
	allMethod string
}

func (check capabilityEndpointCheck) Name() string { return "regex" }

func (check capabilityEndpointCheck) Authorized(capability, path, method string) bool {
	parts := check.prefix.FindStringSubmatch(capability)
	if len(parts) < 3 || (parts[2] != check.allMethod && parts[2] != strings.ToLower(method)) {
		return false
	}
	pattern, err := regexp.Compile(parts[1])
	if err != nil {
		return false
	}
	match := pattern.FindStringIndex(path)
	return len(match) > 0 && match[0] == 0
}

type capabilityMetricValidator struct {
	checker   basculechecks.CapabilitiesValidator
	counter   metrics.Counter
	endpoints []*regexp.Regexp
	enforce   bool
}

func (validator capabilityMetricValidator) Check(ctx context.Context, _ bascule.Token) error {
	auth, ok := bascule.FromContext(ctx)
	client, partner, endpoint, reason := "", "", "", ""
	var err error
	if !ok {
		reason, err = "auth_missing", basculechecks.ErrNoAuth
	} else {
		client, partner, endpoint, reason, err = validator.checkAuthentication(auth)
	}
	outcome := "accepted"
	if err != nil && validator.enforce {
		outcome = "rejected"
	}
	validator.counter.With("outcome", outcome, "reason", reason, "clientid", client, "partnerid", partner, "endpoint", endpoint).Add(1)
	if validator.enforce {
		return err
	}
	return nil
}

func (validator capabilityMetricValidator) checkAuthentication(auth bascule.Authentication) (client, partner, endpoint, reason string, err error) {
	if auth.Token == nil {
		return "", "", "", reasonMissingValues, basculechecks.ErrNoToken
	}
	client = auth.Token.Principal()
	if auth.Token.Attributes() == nil {
		return client, "", "", reasonMissingValues, basculechecks.ErrNilAttributes
	}
	value, ok := bascule.GetNestedAttribute(auth.Token.Attributes(), basculechecks.PartnerKeys()...)
	if !ok {
		return client, "", "", reasonUndeterminedID, basculechecks.ErrGettingPartnerIDs
	}
	partners, err := cast.ToStringSliceE(value)
	if err != nil {
		return client, "", "", reasonUndeterminedID, err
	}
	partner = basculechecks.DeterminePartnerMetric(partners)
	if auth.Request.URL == nil {
		return client, partner, "", reasonMissingValues, basculechecks.ErrNoURL
	}
	endpoint = "not_recognized"
	for _, pattern := range validator.endpoints {
		match := pattern.FindStringIndex(auth.Request.URL.EscapedPath())
		if len(match) > 0 && match[0] == 0 {
			endpoint = pattern.String()
			break
		}
	}
	err = validator.checker.CheckAuthentication(auth, basculechecks.ParsedValues{Endpoint: endpoint})
	if err != nil {
		reason = "unknown"
		var reasoner basculechecks.Reasoner
		if errors.As(err, &reasoner) {
			reason = reasoner.Reason()
		}
	}
	return
}
