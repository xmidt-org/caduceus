package main

import (
	"testing"

	"github.com/go-kit/kit/metrics/provider"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/webpa-common/v2/logging"
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

func TestNewPrimaryHandler(t *testing.T) {
	var (
		l                  = logging.New(nil)
		viper              = viper.New()
		sw                 = &ServerHandler{}
		expectedAuthHeader = []string{"Basic xxxxxxx"}
	)
	r, err := xmetrics.NewRegistry(nil)
	require.NoError(t, err)

	viper.Set("authHeader", expectedAuthHeader)
	c := ancla.HandlerConfig{
		MetricsProvider: provider.NewDiscardProvider(),
	}
	if _, err := NewPrimaryHandler(l, viper, r, sw, nil, c, mux.NewRouter()); err != nil {
		t.Fatalf("NewPrimaryHandler failed: %v", err)
	}

}
