//
// Copyright 2026 Derek Handy
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Project can be found at: https://github.com/derekhandy/ngen-laser
//

package main

import "testing"

func TestRunReturnsFailureExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing command", args: []string{"laser"}},
		{name: "gui without command", args: []string{"laser", "-gui"}},
		{name: "missing train path", args: []string{"laser", "train"}},
		{name: "missing pack path", args: []string{"laser", "pack"}},
		{name: "invalid train path", args: []string{"laser", "train", "missing-path"}},
		{name: "invalid pack path", args: []string{"laser", "pack", "missing-path"}},
		{name: "invalid compression arguments", args: []string{"laser", "pack", "-c"}},
		{name: "missing unpack path", args: []string{"laser", "unpack"}},
		{name: "unknown command", args: []string{"laser", "unknown"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := run(test.args); got == 0 {
				t.Fatalf("run(%v) returned success for invalid input", test.args)
			}
		})
	}
}
