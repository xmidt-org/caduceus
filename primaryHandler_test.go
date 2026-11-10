// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/gorilla/mux"
	metricdto "github.com/prometheus/client_model/go"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"
	"github.com/xmidt-org/webpa-common/v2/adapter"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

func TestNewPrimaryHandler(t *testing.T) {
	var (
		l                  = adapter.DefaultLogger().Logger
		viper              = viper.New()
		sw                 = &ServerHandler{}
		expectedAuthHeader = []string{"Basic xxxxxxx"}
	)
	require := require.New(t)
	r, err := xmetrics.NewRegistry(nil)
	require.NoError(err)
	cfg := touchstone.Config{
		DefaultNamespace: "n",
		DefaultSubsystem: "s",
	}
	_, pr, err := touchstone.New(cfg)
	require.NoError(err)

	tf := touchstone.NewFactory(cfg, sallust.Default(), pr)
	require.NotNil(tf)

	viper.Set("authHeader", expectedAuthHeader)
	if _, err := NewPrimaryHandler(l, viper, r, tf, sw, nil, mux.NewRouter(), true); err != nil {
		t.Fatalf("NewPrimaryHandler failed: %v", err)
	}

}

func TestAuthenticationCompatibility(t *testing.T) {
	config := viper.New()
	config.Set("authHeader", []string{base64.StdEncoding.EncodeToString([]byte("client:secret"))})
	router, _ := authenticationTestRouter(t, config)
	for _, version := range []string{"v3", "v4"} {
		for _, test := range []struct {
			name   string
			auth   string
			status int
		}{
			{name: "valid basic", auth: "Basic " + base64.StdEncoding.EncodeToString([]byte("client:secret")), status: http.StatusNoContent},
			{name: "invalid basic", auth: "Basic " + base64.StdEncoding.EncodeToString([]byte("client:wrong")), status: http.StatusUnauthorized},
			{name: "missing auth", status: http.StatusUnauthorized},
			{name: "malformed basic", auth: "Basic !!!", status: http.StatusUnauthorized},
			{name: "unsupported scheme", auth: "Digest token", status: http.StatusUnauthorized},
			{name: "malformed header", auth: "Bearer", status: http.StatusBadRequest},
		} {
			t.Run(version+"/"+test.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodPost, "/api/"+version+"/notify", nil)
				if test.auth != "" {
					request.Header.Set("Authorization", test.auth)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				expectedStatus := test.status
				if version == "v3" && expectedStatus == http.StatusUnauthorized {
					expectedStatus = http.StatusForbidden
				} else if version == "v4" && expectedStatus == http.StatusBadRequest {
					expectedStatus = http.StatusUnauthorized
				}
				require.Equal(t, expectedStatus, response.Code)
				if version == "v4" && expectedStatus == http.StatusUnauthorized {
					require.Equal(t, "Bearer", response.Header().Get("WWW-Authenticate"))
				} else {
					require.Empty(t, response.Header().Get("WWW-Authenticate"))
				}
				require.Empty(t, response.Body.String())
			})
		}
	}
}

func authenticationTestRouter(t *testing.T, config *viper.Viper) (*mux.Router, xmetrics.Registry) {
	t.Helper()
	registry, err := xmetrics.NewRegistry(nil, authCapabilitiesMetrics, authValidationMetrics)
	require.NoError(t, err)
	metricConfig := touchstone.Config{DefaultNamespace: "n", DefaultSubsystem: "s"}
	_, prometheusRegistry, err := touchstone.New(metricConfig)
	require.NoError(t, err)
	factory := touchstone.NewFactory(metricConfig, sallust.Default(), prometheusRegistry)
	chain, err := authenticationMiddleware(config, adapter.DefaultLogger().Logger, registry, factory)
	require.NoError(t, err)
	router := mux.NewRouter()
	router.Handle("/api/{version:v3|v4}/notify", chain.ThenFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	return router, registry
}

func TestJWTAuthenticationCompatibility(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	publicKey := map[string]interface{}{
		"kty": "RSA", "kid": defaultKeyID,
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
	keyServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/"+defaultKeyID {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		json.NewEncoder(response).Encode(publicKey)
	}))
	t.Cleanup(keyServer.Close)
	now := time.Now().Unix()
	for _, test := range []struct {
		name       string
		mode       string
		claims     jwt.MapClaims
		leeway     map[string]int64
		method     jwt.SigningMethod
		wrongKey   bool
		omitKeyID  bool
		unknownKey bool
		status     int
		reason     string
	}{
		{name: "valid", mode: "enforce", status: http.StatusNoContent},
		{name: "rsa pss", mode: "enforce", method: jwt.SigningMethodPS256, status: http.StatusNoContent},
		{name: "default key ID", mode: "enforce", omitKeyID: true, status: http.StatusNoContent},
		{name: "unknown key", unknownKey: true, status: http.StatusUnauthorized},
		{name: "bad signature", mode: "monitor", wrongKey: true, status: http.StatusUnauthorized},
		{name: "unsigned token", method: jwt.SigningMethodNone, status: http.StatusUnauthorized},
		{name: "expired", claims: jwt.MapClaims{"exp": now - 60}, status: http.StatusUnauthorized},
		{name: "not yet valid", claims: jwt.MapClaims{"nbf": now + 60}, status: http.StatusUnauthorized},
		{name: "future issued at", claims: jwt.MapClaims{"iat": now + 60}, status: http.StatusUnauthorized},
		{name: "expiry leeway", claims: jwt.MapClaims{"exp": now - 60}, leeway: map[string]int64{"exp": -120}, status: http.StatusNoContent},
		{name: "not before leeway", claims: jwt.MapClaims{"nbf": now + 60}, leeway: map[string]int64{"nbf": -120}, status: http.StatusNoContent},
		{name: "issued at leeway", claims: jwt.MapClaims{"iat": now + 60}, leeway: map[string]int64{"iat": -120}, status: http.StatusNoContent},
		{name: "missing principal", claims: jwt.MapClaims{"sub": nil}, status: http.StatusUnauthorized},
		{name: "empty principal", claims: jwt.MapClaims{"sub": ""}, status: http.StatusForbidden},
		{name: "disabled capabilities", claims: jwt.MapClaims{"capabilities": nil, "allowedResources": nil}, status: http.StatusNoContent},
		{name: "monitor missing capabilities", mode: "monitor", claims: jwt.MapClaims{"capabilities": nil}, status: http.StatusNoContent, reason: "undetermined_capabilities"},
		{name: "enforce missing capabilities", mode: "enforce", claims: jwt.MapClaims{"capabilities": nil}, status: http.StatusForbidden, reason: "undetermined_capabilities"},
		{name: "empty capabilities", mode: "enforce", claims: jwt.MapClaims{"capabilities": []string{}}, status: http.StatusForbidden, reason: "empty_capabilities_list"},
		{name: "missing partners", mode: "enforce", claims: jwt.MapClaims{"allowedResources": nil}, status: http.StatusForbidden, reason: "undetermined_partner_ID"},
		{name: "monitor missing partners", mode: "monitor", claims: jwt.MapClaims{"allowedResources": nil}, status: http.StatusNoContent, reason: "undetermined_partner_ID"},
		{name: "wrong endpoint", mode: "enforce", claims: jwt.MapClaims{"capabilities": []string{"xmidt:^other$:post"}}, status: http.StatusForbidden, reason: "no_capabilities_match"},
		{name: "wrong method", mode: "enforce", claims: jwt.MapClaims{"capabilities": []string{"xmidt:^notify$:get"}}, status: http.StatusForbidden, reason: "no_capabilities_match"},
		{name: "invalid regex", mode: "enforce", claims: jwt.MapClaims{"capabilities": []string{"xmidt:[:post"}}, status: http.StatusForbidden, reason: "no_capabilities_match"},
		{name: "unanchored capability", mode: "enforce", claims: jwt.MapClaims{"capabilities": []string{"xmidt:notify:post"}}, status: http.StatusNoContent},
		{name: "all methods", mode: "enforce", claims: jwt.MapClaims{"capabilities": []string{"xmidt:^notify$:all"}}, status: http.StatusNoContent},
		{name: "monitor mismatch", mode: "monitor", claims: jwt.MapClaims{"capabilities": []string{"xmidt:other:post"}}, status: http.StatusNoContent, reason: "no_capabilities_match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := viper.New()
			config.Set("jwtValidator.config.resolve.template", keyServer.URL+"/{keyID}")
			config.Set("capabilityCheck", CapabilityConfig{Type: test.mode, Prefix: "xmidt:", AcceptAllMethod: "all", EndpointBuckets: []string{"notify\\b"}})
			for claim, value := range test.leeway {
				config.Set("jwtValidator.leeway."+claim, value)
			}
			router, registry := authenticationTestRouter(t, config)
			claims := jwt.MapClaims{
				"sub": "client", "exp": now + 3600, "nbf": now - 60, "iat": now - 60,
				"capabilities":     []string{"xmidt:^notify$:post"},
				"allowedResources": map[string]interface{}{"allowedPartners": []string{"partner"}},
			}
			for claim, value := range test.claims {
				if value == nil {
					delete(claims, claim)
				} else {
					claims[claim] = value
				}
			}
			method := test.method
			if method == nil {
				method = jwt.SigningMethodRS256
			}
			token := jwt.NewWithClaims(method, claims)
			if !test.omitKeyID {
				token.Header["kid"] = defaultKeyID
			}
			if test.unknownKey {
				token.Header["kid"] = "unknown"
			}
			var signingKey interface{} = key
			if test.wrongKey {
				signingKey = wrongKey
			} else if method == jwt.SigningMethodNone {
				signingKey = jwt.UnsafeAllowNoneSignatureType
			}
			signed, err := token.SignedString(signingKey)
			require.NoError(t, err)
			for _, version := range []string{"v3", "v4"} {
				request := httptest.NewRequest(http.MethodPost, "/api/"+version+"/notify", nil)
				request.Header.Set("Authorization", "Bearer "+signed)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				expectedStatus := test.status
				if version == "v3" && expectedStatus == http.StatusUnauthorized {
					expectedStatus = http.StatusForbidden
				}
				require.Equal(t, expectedStatus, response.Code, version)
				if version == "v4" && expectedStatus == http.StatusUnauthorized {
					require.Equal(t, "Bearer", response.Header().Get("WWW-Authenticate"))
				} else {
					require.Empty(t, response.Header().Get("WWW-Authenticate"))
				}
				require.Empty(t, response.Body.String())
			}
			outcome := "Accepted"
			switch test.status {
			case http.StatusUnauthorized:
				outcome = "parse_failed"
			case http.StatusForbidden:
				outcome = "checks_failed"
			}
			var metric metricdto.Metric
			require.NoError(t, registry.NewCounterVec("auth_validation").WithLabelValues(outcome).Write(&metric))
			require.Equal(t, float64(2), metric.GetCounter().GetValue())
			if test.mode != "" && test.status != http.StatusUnauthorized {
				capabilityOutcome := "accepted"
				if test.status == http.StatusForbidden {
					capabilityOutcome = "rejected"
				}
				partner, endpoint := "partner", "notify\\b"
				if test.reason == "undetermined_partner_ID" {
					partner, endpoint = "", ""
				}
				require.NoError(t, registry.NewCounterVec("auth_capability_check").WithLabelValues(capabilityOutcome, test.reason, "client", partner, endpoint).Write(&metric))
				require.Equal(t, float64(2), metric.GetCounter().GetValue())
			}
		})
	}
}
