package main

import (
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/logging"
	// nolint:staticcheck
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
	if _, err := NewPrimaryHandler(l, viper, r, sw, nil, mux.NewRouter(), true); err != nil {
		t.Fatalf("NewPrimaryHandler failed: %v", err)
	}

}
