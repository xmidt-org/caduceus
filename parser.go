/**
 * Copyright 2020 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/xmidt-org/webpa-common/logging"
	wrpparser "github.com/xmidt-org/wrp-listener/wrpParser"
)

var (
	nopLogger            = log.NewNopLogger()
	defaultDefaultFinder = wrpparser.FieldFinder{Field: wrpparser.Source}
)

func setupParser(config map[string]ParserConfig, logger log.Logger) (*wrpparser.StrParser, error) {
	cs := []wrpparser.Classifier{}
	os := []wrpparser.ParserOption{}
	var defaultFinder wrpparser.DeviceFinder

	// So we don't have to check before every log statement.
	if logger == nil {
		logger = nopLogger
	}

	for k, v := range config {
		// we should handle the default differently
		if k == "default" {
			field := wrpparser.GetField(v.DeviceLocation.Field)
			defaultFinder = wrpparser.FieldFinder{Field: field}
			if v.DeviceLocation.Regex != "" && v.DeviceLocation.RegexLabel != "" {
				f, err := wrpparser.NewRegexpFinderFromStr(
					field,
					v.DeviceLocation.Regex,
					v.DeviceLocation.RegexLabel)
				if err != nil {
					logger.Log(level.Key(), level.ErrorValue(),
						logging.MessageKey(), "Failed to create regexp default finder, creating a field finder instead",
						logging.ErrorKey(), err,
						"field", field,
						"failed regexp string", v.DeviceLocation.Regex,
						"failed regexp label", v.DeviceLocation.RegexLabel)
				} else {
					// if it's successful then use the regexp finder
					defaultFinder = f
				}
			}
		} else {
			// set up the classifier, so we can get to the finder with the associated label.
			c, err := wrpparser.NewRegexpClassifierFromStr(k, v.Regex, wrpparser.GetField(v.Field))
			if err != nil {
				logger.Log(level.Key(), level.ErrorValue(),
					logging.MessageKey(), "Failed to create regexp classifier, not including classifier",
					logging.ErrorKey(), err,
					"label", k,
					"failed regexp string", v.Regex)
			} else {
				cs = append(cs, c)
			}

			// set up the finder for this label
			var f wrpparser.DeviceFinder
			field := wrpparser.GetField(v.DeviceLocation.Field)
			f = wrpparser.FieldFinder{Field: field}
			if v.DeviceLocation.Regex != "" && v.DeviceLocation.RegexLabel != "" {
				f, err = wrpparser.NewRegexpFinderFromStr(
					field,
					v.DeviceLocation.Regex,
					v.DeviceLocation.RegexLabel)
				if err != nil {
					logger.Log(level.Key(), level.ErrorValue(),
						logging.MessageKey(), "Failed to create regexp finder, creating a field finder instead",
						logging.ErrorKey(), err,
						"field", field,
						"failed regexp string", v.DeviceLocation.Regex,
						"failed regexp label", v.DeviceLocation.RegexLabel)
					f = wrpparser.FieldFinder{Field: field}
				}
			}
			os = append(os, wrpparser.WithDeviceFinder(k, f))
		}
	}

	var (
		classifier wrpparser.Classifier
		err        error
	)
	classifier, err = wrpparser.NewMultClassifier(cs...)
	if err != nil {
		// this will force the parser to always use the default finder
		classifier = wrpparser.NewConstClassifier("", false)
	}

	// make sure we have a default finder set
	if defaultFinder == nil {
		defaultFinder = defaultDefaultFinder
	}

	// Actually create our device id parser, now that we have all of its
	// dependencies.  There is no unit test for this error, because it
	// shouldn't be possible; the only time an error is returned is if the
	// classifier or default finder are nil, and previous checks ensure this
	// won't happen.
	deviceIDParser, err := wrpparser.NewStrParser(classifier, defaultFinder, os...)
	if err != nil {
		return nil, fmt.Errorf("failed to create new string parser from configuration: [%w]", err)
	}
	return deviceIDParser, nil
}
