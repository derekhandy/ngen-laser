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

func TestAllOperandSpellingsPopulatesEchoCandidates(t *testing.T) {
	operands := AllOperandSpellingsForLength(&Rollout{}, 20, 10)
	seen := make(map[string]bool)
	var echoOperands []string
	for _, operand := range operands {
		if operand == "" {
			t.Fatal("operand list contains an empty candidate")
		}
		if len(operand) > 0 && operand[0] == 'e' {
			echoOperands = append(echoOperands, operand)
			if seen[operand] {
				t.Fatalf("duplicate echo candidate %q", operand)
			}
			seen[operand] = true
		}
	}
	want := []string{"e1", "e2", "e3", "e4", "e5", "e6", "e7", "e8", "e9"}
	if len(echoOperands) != len(want) {
		t.Fatalf("echo candidates = %v, want %v", echoOperands, want)
	}
	for i := range want {
		if echoOperands[i] != want[i] {
			t.Fatalf("echo candidates = %v, want %v", echoOperands, want)
		}
	}
}

func TestComputeInstructionsRunsExhaustiveWorkers(t *testing.T) {
	env := NewCompressionEnvironment(";;;", &Rollout{})
	if _, _, err := ComputeInstructions(env, nil, []int{0, 0}, 1, 1); err != nil {
		t.Fatal(err)
	}
}
