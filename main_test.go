// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	_ "github.com/goschtalt/goschtalt/pkg/typical"
	_ "github.com/goschtalt/yaml-decoder"
	_ "github.com/goschtalt/yaml-encoder"
	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/sallust"
)

func Test_provideCLI(t *testing.T) {
	tests := []struct {
		description string
		args        cliArgs
		want        CLI
		exits       bool
		expectedErr error
	}{
		{
			description: "no arguments, everything works",
		}, {
			description: "dev mode",
			args:        cliArgs{"-d"},
			want:        CLI{Dev: true},
		}, {
			description: "invalid argument",
			args:        cliArgs{"-w"},
			exits:       true,
		}, {
			description: "invalid argument",
			args:        cliArgs{"-d", "-w"},
			exits:       true,
		}, {
			description: "help",
			args:        cliArgs{"-h"},
			exits:       true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)

			if tc.exits {
				assert.Panics(func() {
					_, _ = provideCLIWithOpts(tc.args, true)
				})
			} else {
				got, err := provideCLI(tc.args)

				assert.ErrorIs(err, tc.expectedErr)
				want := tc.want
				assert.Equal(&want, got)
			}
		})
	}
}

func Test_caduceus(t *testing.T) {
	tests := []struct {
		description string
		args        []string
		panic       bool
		expectedErr error
	}{
		{
			description: "show config and exit",
			args:        []string{"-s"},
			panic:       true,
		}, {
			description: "show help and exit",
			args:        []string{"-h"},
			panic:       true,
		}, {
			description: "do everything but run",
			args:        []string{"-f", "minimal.yaml"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)

			if tc.panic {
				assert.Panics(func() {
					_ = caduceus(tc.args, false)
				})
			} else {
				err := caduceus(tc.args, false)

				assert.ErrorIs(err, tc.expectedErr)
			}
		})
	}
}

func Test_provideLogger(t *testing.T) {
	tests := []struct {
		description string
		cli         *CLI
		cfg         sallust.Config
		expectedErr error
	}{
		{
			description: "validate empty config",
			cfg:         sallust.Config{},
			cli:         &CLI{},
		}, {
			description: "validate dev config",
			cfg:         sallust.Config{},
			cli:         &CLI{Dev: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)

			got, err := provideLogger(tc.cli, tc.cfg)

			if tc.expectedErr == nil {
				assert.NotNil(got)
				assert.NoError(err)
				return
			}
			assert.ErrorIs(err, tc.expectedErr)
			assert.Nil(got)
		})
	}
}
