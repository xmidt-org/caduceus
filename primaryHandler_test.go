// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"

	"github.com/gorilla/mux"
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
