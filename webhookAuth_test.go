// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/bascule/acquire"
)

func signedJWT(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("key"))
	require.NoError(t, err)
	return signed
}

func TestNewJWTAcquireParser(t *testing.T) {
	for _, parserType := range []string{"", "simple"} {
		parser, err := newJWTAcquireParser(parserType)
		require.NoError(t, err)
		assert.Nil(t, parser.token, parserType)
		assert.Nil(t, parser.expiration, parserType)
	}

	parser, err := newJWTAcquireParser("raw")
	require.NoError(t, err)
	assert.NotNil(t, parser.token)
	assert.NotNil(t, parser.expiration)

	_, err = newJWTAcquireParser("other")
	assert.Error(t, err)
}

func TestRawTokenParser(t *testing.T) {
	token, err := rawTokenParser([]byte("abc.def.ghi"))
	require.NoError(t, err)
	assert.Equal(t, "abc.def.ghi", token)
}

func TestRawTokenExpirationParser(t *testing.T) {
	expiry := time.Now().Add(time.Hour).Unix()

	t.Run("valid", func(t *testing.T) {
		exp, err := rawTokenExpirationParser([]byte(signedJWT(t, jwt.MapClaims{"exp": expiry})))
		require.NoError(t, err)
		assert.Equal(t, time.Unix(expiry, 0), exp)
	})

	t.Run("expired tokens still parse", func(t *testing.T) {
		past := time.Now().Add(-time.Hour).Unix()
		exp, err := rawTokenExpirationParser([]byte(signedJWT(t, jwt.MapClaims{"exp": past})))
		require.NoError(t, err)
		assert.Equal(t, time.Unix(past, 0), exp)
	})

	t.Run("not a jwt", func(t *testing.T) {
		_, err := rawTokenExpirationParser([]byte("garbage"))
		assert.Error(t, err)
	})

	t.Run("missing exp", func(t *testing.T) {
		_, err := rawTokenExpirationParser([]byte(signedJWT(t, jwt.MapClaims{"sub": "x"})))
		assert.ErrorIs(t, err, errMissingExpClaim)
	})

	t.Run("non numeric exp", func(t *testing.T) {
		_, err := rawTokenExpirationParser([]byte(signedJWT(t, jwt.MapClaims{"exp": "soon"})))
		assert.Error(t, err)
	})
}

func jwtOptionsFor(url string) acquire.RemoteBearerTokenAcquirerOptions {
	return acquire.RemoteBearerTokenAcquirerOptions{AuthURL: url, Buffer: time.Minute, Timeout: 5 * time.Second}
}

func TestArgusAcquirerSelection(t *testing.T) {
	t.Run("nothing configured", func(t *testing.T) {
		a, err := argusAcquirer("", ArgusAuth{})
		require.NoError(t, err)
		assert.IsType(t, &acquire.DefaultAcquirer{}, a)
	})

	t.Run("basic", func(t *testing.T) {
		a, err := argusAcquirer("", ArgusAuth{Basic: "Basic dXNlcjpwYXNz"})
		require.NoError(t, err)
		assert.IsType(t, &acquire.FixedValueAcquirer{}, a)
	})

	t.Run("jwt", func(t *testing.T) {
		a, err := argusAcquirer("raw", ArgusAuth{JWT: jwtOptionsFor("http://auth.example.com")})
		require.NoError(t, err)
		assert.IsType(t, &acquire.RemoteBearerTokenAcquirer{}, a)
	})

	t.Run("jwt wins over basic", func(t *testing.T) {
		a, err := argusAcquirer("", ArgusAuth{JWT: jwtOptionsFor("http://auth.example.com"), Basic: "Basic x"})
		require.NoError(t, err)
		assert.IsType(t, &acquire.RemoteBearerTokenAcquirer{}, a)
	})

	// Missing buffer or timeout means JWT is not configured, as with ancla 0.3.x.
	t.Run("incomplete jwt falls back to basic", func(t *testing.T) {
		a, err := argusAcquirer("", ArgusAuth{
			JWT:   acquire.RemoteBearerTokenAcquirerOptions{AuthURL: "http://auth.example.com"},
			Basic: "Basic x",
		})
		require.NoError(t, err)
		assert.IsType(t, &acquire.FixedValueAcquirer{}, a)
	})

	t.Run("bad parser type", func(t *testing.T) {
		_, err := argusAcquirer("other", ArgusAuth{JWT: jwtOptionsFor("http://auth.example.com")})
		assert.Error(t, err)
		_, err = argusAuthDecorator("other", ArgusAuth{JWT: jwtOptionsFor("http://auth.example.com")})
		assert.Error(t, err)
	})
}

func decorate(t *testing.T, parserType string, auth ArgusAuth) (*http.Request, error) {
	t.Helper()
	decorator, err := argusAuthDecorator(parserType, auth)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "http://argus.example.com", nil)
	return request, decorator.Decorate(context.Background(), request)
}

func TestArgusAuthDecorator(t *testing.T) {
	t.Run("no auth adds no header", func(t *testing.T) {
		request, err := decorate(t, "", ArgusAuth{})
		require.NoError(t, err)
		assert.Empty(t, request.Header.Get("Authorization"))
	})

	t.Run("basic", func(t *testing.T) {
		request, err := decorate(t, "", ArgusAuth{Basic: "Basic dXNlcjpwYXNz"})
		require.NoError(t, err)
		assert.Equal(t, "Basic dXNlcjpwYXNz", request.Header.Get("Authorization"))
	})

	t.Run("raw jwt", func(t *testing.T) {
		token := signedJWT(t, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()})
		var requestHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestHeader = r.Header.Get("X-Client")
			w.Write([]byte(token))
		}))
		t.Cleanup(server.Close)

		options := jwtOptionsFor(server.URL)
		options.RequestHeaders = map[string]string{"X-Client": "caduceus"}
		request, err := decorate(t, "raw", ArgusAuth{JWT: options})
		require.NoError(t, err)
		assert.Equal(t, "Bearer "+token, request.Header.Get("Authorization"))
		assert.Equal(t, "caduceus", requestHeader)
	})

	t.Run("simple jwt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"serviceAccessToken":"abc","expires_in":3600}`))
		}))
		t.Cleanup(server.Close)

		request, err := decorate(t, "simple", ArgusAuth{JWT: jwtOptionsFor(server.URL)})
		require.NoError(t, err)
		assert.Equal(t, "Bearer abc", request.Header.Get("Authorization"))
	})

	t.Run("token endpoint failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)

		_, err := decorate(t, "raw", ArgusAuth{JWT: jwtOptionsFor(server.URL)})
		assert.Error(t, err)
	})
}
