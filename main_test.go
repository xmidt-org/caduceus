/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
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
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// func TestPrintVersionInfo(t *testing.T) {
// 	testCases := []struct {
// 		name           string
// 		expectedOutput []string
// 		overrideValues func()
// 		lineCount      int
// 	}{
// 		{
// 			"default",
// 			[]string{
// 				"caduceus:",
// 				"version: \tundefined",
// 				"go version: \tgo",
// 				"built time: \tundefined",
// 				"git commit: \tundefined",
// 				"os/arch: \t",
// 			},
// 			func() {},
// 			6,
// 		},
// 		{
// 			"set values",
// 			[]string{
// 				"caduceus:",
// 				"version: \t1.0.0\n",
// 				"go version: \tgo",
// 				"built time: \tsome time\n",
// 				"git commit: \tgit sha\n",
// 				"os/arch: \t",
// 			},
// 			func() {
// 				Version = "1.0.0"
// 				BuildTime = "some time"
// 				GitCommit = "git sha"
// 			},
// 			6,
// 		},
// 	}
// 	for _, tc := range testCases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			resetGlobals()
// 			tc.overrideValues()
// 			buf := &bytes.Buffer{}
// 			printVersionInfo(buf)
// 			count := 0
// 			for {
// 				line, err := buf.ReadString(byte('\n'))
// 				if err != nil {
// 					break
// 				}
// 				assert.Contains(t, line, tc.expectedOutput[count])
// 				if strings.Contains(line, "\t") {
// 					keyAndValue := strings.Split(line, "\t")
// 					// The value after the tab should have more than 2 characters
// 					// 1) the first character of the value and the new line
// 					assert.True(t, len(keyAndValue[1]) > 2)
// 				}
// 				count++
// 			}
// 			assert.Equal(t, tc.lineCount, count)
// 			resetGlobals()
// 		})
// 	}
// }

// func resetGlobals() {
// 	Version = "undefined"
// 	BuildTime = "undefined"
// 	GitCommit = "undefined"
// }
