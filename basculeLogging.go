package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/sallust"
	"go.uber.org/zap"
)

func sanitizeHeaders(headers http.Header) (filtered http.Header) {
	filtered = headers.Clone()
	if authHeader := filtered.Get("Authorization"); authHeader != "" {
		filtered.Del("Authorization")
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 {
			filtered.Set("Authorization-Type", parts[0])
		}
	}
	return
}

func setLogger(logger *zap.Logger) func(delegate http.Handler) http.Handler {
	return func(delegate http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				kvs := []interface{}{"requestHeaders", sanitizeHeaders(r.Header), "requestURL", r.URL.EscapedPath(), "method", r.Method}
				kvs, _ = candlelight.AppendTraceInfo(r.Context(), kvs)
				ctx := r.WithContext(sallust.With(r.Context(), logger))
				delegate.ServeHTTP(w, ctx)
			})
	}
}

func getLogger(ctx context.Context) *zap.Logger {
	logger := sallust.Get(ctx).With(zap.Time("ts", time.Now().UTC()), zap.Any("caller", zap.WithCaller(true)))
	return logger
}