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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/wrp-go/v3"
)

func TestSetupParser(t *testing.T) {
	tests := []struct {
		description string
		parsers     map[string]ParserConfig
		wrpsToParse []*wrp.Message
		expectedIDs []string
	}{
		{
			description: "all defaults",
		},
		{
			description: "regexp classifier creation failed",
		}
		{
			description: "regexp finders creation failed",
		},
		{
			description: "multiple classifiers in the right order",
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			require.Equal(t, len(tc.wrpsToParse), len(tc.expectedIDs))
			assert := assert.New(t)

			parser, err := setupParser(tc.parsers, nil)
			assert.Nil(err)

			// verify that the message will be parsed the way we expect given
			// the classifiers and finders created.
			for i, msg := range tc.wrpsToParse {
				id, err := parser.Parse(msg)
				assert.Nil(err)
				assert.Equal(tc.expectedIDs[i], id)
			}
		})
	}
}

func TestSetupParserParseErr(t *testing.T) {
	// The only possible error given the current setup of the deviceIDParser is
	// on Parse().  This can happen if the default finder is a regexp finder,
	// and the finder's regexp doesn't match anything in the field given.

	// set up a parser with a regexp default finder, then pass a message who
	// doesn't match to get an error on Parse().
}
