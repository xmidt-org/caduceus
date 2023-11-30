// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: LicenseRef-COMCAST

package main

import (
	"github.com/xmidt-org/sallust"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Create the logger and configure it based on if the program is in
// debug mode or normal mode.
func provideLogger(cli *CLI, cfg sallust.Config) (*zap.Logger, error) {
	if cli.Dev {
		cfg.Level = "DEBUG"
		cfg.Development = true
		cfg.Encoding = "console"
		cfg.EncoderConfig = sallust.EncoderConfig{
			TimeKey:        "T",
			LevelKey:       "L",
			NameKey:        "N",
			CallerKey:      "C",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "M",
			StacktraceKey:  "S",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    "capitalColor",
			EncodeTime:     "RFC3339",
			EncodeDuration: "string",
			EncodeCaller:   "short",
		}
		cfg.OutputPaths = []string{"stderr"}
		cfg.ErrorOutputPaths = []string{"stderr"}
	}
	return cfg.Build()
}
