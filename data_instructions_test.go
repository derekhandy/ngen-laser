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

import (
	"testing"
)

func withOperandLineLength(t *testing.T, length int) {
	t.Helper()
	previous := operandLineLength
	UpdateOperandLength(length)
	t.Cleanup(func() {
		UpdateOperandLength(previous)
	})
}

func TestInstructionsUseIndependentRecursionGuards(t *testing.T) {
	first := NewInstructions()
	second := NewInstructions()

	if !first.guard.Enter("shared-key") {
		t.Fatal("first evaluator could not enter its guard")
	}
	defer first.guard.Exit("shared-key")
	if !second.guard.Enter("shared-key") {
		t.Fatal("second evaluator was affected by first evaluator state")
	}
	defer second.guard.Exit("shared-key")

	if !second.guard.Enter("shared-key") {
		t.Fatal("resetting one evaluator affected another evaluator")
	}
	second.guard.Exit("shared-key")
}

func TestOperandLineLengthTwoConsumesExactlyTwelveDigits(t *testing.T) {
	withOperandLineLength(t, 2)

	interpreted := NewICommands().ReturnInterpret(",,,,,,,,,,,,!tu------------", 0, &[]IndexEntry{})
	if interpreted != ",,,,,,,,,,,,tu" {
		t.Fatalf("expected tu to consume exactly 12 right-hand digits, got %q", interpreted)
	}
}

func TestDefaultOperandLineLengthRendersTranslateAcrossTwoLines(t *testing.T) {
	withOperandLineLength(t, 2)

	rendered := NewICommands().ReturnRendered(",,,,,,,,,,,,tu")
	want := ",,,,,,,,,,,,,-,,-,,-,,-,"
	if rendered != want {
		t.Fatalf("expected tu to render two translated lines, got %q want %q", rendered, want)
	}
}

func TestOperandLineLengthScalesCoordinateAndStringOperands(t *testing.T) {
	withOperandLineLength(t, 3)

	interpreted := NewICommands().ReturnInterpret(",,,,,,,,,,,,,,,,,,!tu------------------", 0, &[]IndexEntry{})
	if interpreted != ",,,,,,,,,,,,,,,,,,tu" {
		t.Fatalf("expected tu to consume 18 right-hand digits, got %q", interpreted)
	}

	rendered := NewICommands().ReturnRendered(",,,,,,......------l")
	want := ",,,,,,......------,,,,,,......------"
	if rendered != want {
		t.Fatalf("expected l to sort each 6-digit line independently, got %q want %q", rendered, want)
	}
}
