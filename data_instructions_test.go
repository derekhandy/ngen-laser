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
	"bytes"
	"strconv"
	"strings"
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

func TestTwoDigitMagnitudeEcho(t *testing.T) {
	inst := NewInstructions()

	// Build a source where the same 15-glyph region appears twice in a row.
	source := strings.Repeat("-,.;", 5)[:15]
	original := source + source

	// Insert !e15 at the boundary.
	modified := original[:15] + "!e15" + original[15:]

	interpreted := inst.Interpret(modified, 0, &[]IndexEntry{})
	if interpreted == "" || interpreted == "+" {
		t.Fatalf("interpret failed on %q", modified)
	}

	index := []IndexEntry{}
	rendered := inst.Render(interpreted, 0, &index)
	if rendered != original {
		t.Fatalf("e15 round-trip:\n  original    %q\n  interpreted %q\n  rendered    %q",
			original, interpreted, rendered)
	}
}

func TestTwoDigitMagnitudeBoundary(t *testing.T) {
	inst := NewInstructions()

	for _, v := range []int{9, 10, 11, 30, 31} {
		source := strings.Repeat("-", v)
		original := source + source
		modified := original[:v] + "!e" + strconv.Itoa(v) + original[v:]

		interpreted := inst.Interpret(modified, 0, &[]IndexEntry{})
		if interpreted == "" || interpreted == "+" {
			t.Fatalf("v=%d: interpret failed", v)
		}

		index := []IndexEntry{}
		rendered := inst.Render(interpreted, 0, &index)
		if rendered != original {
			t.Fatalf("v=%d: round-trip failed:\n  original %q\n  rendered %q",
				v, original, rendered)
		}
	}
}

func TestSwapRoundTrip(t *testing.T) {
	inst := NewInstructions()

	// Original glyph stream: 9 glyphs = 3 coordinates.
	original := "---,,,;;;"

	// Network inserts !s3 at the boundary between the two 3-glyph runs.
	modified := original[:3] + "!s3" + original[3:]

	interpreted := inst.Interpret(modified, 0, &[]IndexEntry{})
	if interpreted == "" || interpreted == "+" {
		t.Fatalf("interpret failed on %q", modified)
	}

	index := []IndexEntry{}
	rendered := inst.Render(interpreted, 0, &index)
	if rendered == "" || rendered == "+" {
		t.Fatalf("render failed on %q", interpreted)
	}

	// Lossless invariant: render(interpret(mod)) must reproduce the
	// original glyph stream. The swap is a rearrangement; render undoes
	// it to recover the bytes the network was compressing.
	if rendered != original {
		t.Fatalf("swap round-trip:\n  original    %q\n  interpreted %q\n  rendered    %q",
			original, interpreted, rendered)
	}
}

func TestExtendedOperandRoundTrip(t *testing.T) {
	for _, mag := range []uint64{1, 2, 3, 8, 9} {
		tok := PacketToken{
			Kind:      PacketTokenMagnitudeOperand,
			Op:        's',
			Magnitude: mag,
		}
		enc := EncodePacketOperandToken(tok)
		if len(enc) < 2 {
			t.Fatalf("magnitude %d: encoded too short: %v", mag, enc)
		}
		if enc[0] != compactOperandExtended {
			t.Fatalf("magnitude %d: first byte 0x%02x, want 0x%02x", mag, enc[0], compactOperandExtended)
		}
		r := bytes.NewReader(enc[1:])
		got, err := ReadPacketOperandToken(r, compactOperandExtended)
		if err != nil {
			t.Fatalf("magnitude %d: decode: %v", mag, err)
		}
		if got.Op != 's' {
			t.Fatalf("magnitude %d: got op %q, want 's'", mag, got.Op)
		}
		if got.Magnitude != mag {
			t.Fatalf("magnitude %d: got %d", mag, got.Magnitude)
		}
	}
}

func TestDefaultOperandLineLengthRendersTranslateAcrossTwoLines(t *testing.T) {
	UpdateOperandLength(2)

	rendered := NewICommands().ReturnRendered(",,,,,,,,,,,,tu")
	want := ",,,,,,,,,,,,,-,,-,,-,,-,"
	if rendered != want {
		t.Fatalf("expected tu to render two translated lines, got %q want %q", rendered, want)
	}
}

func TestOperandLineLengthScalesCoordinateAndStringOperands(t *testing.T) {
	UpdateOperandLength(3)

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

func TestTwoDigitMagnitudeRoundTrip(t *testing.T) {
	inst := NewInstructions()

	for _, v := range []int{1, 9, 10, 15, 25, 31} {
		source := strings.Repeat("-,.;", 11)[:v+((v+2)/3*3-v)] // v glyphs, padded to multiple of 3
		for len(source) < v {
			source += "-"
		}
		source = source[:v]
		if v%3 != 0 {
			source = source[:v-v%3]
			continue
		}
		original := source + source
		modified := original[:v] + "!e" + strconv.Itoa(v) + original[v:]

		interpreted := inst.Interpret(modified, 0, &[]IndexEntry{})
		if interpreted == "" || interpreted == "+" {
			t.Fatalf("v=%d: interpret failed", v)
		}

		index := []IndexEntry{}
		rendered := inst.Render(interpreted, 0, &index)
		if rendered != original {
			t.Fatalf("v=%d: round-trip failed:\n  original    %q\n  interpreted %q\n  rendered    %q",
				v, original, interpreted, rendered)
		}
	}
}
