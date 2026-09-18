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
	"testing"
)

func TestMagnitudeOperandRoundTrip(t *testing.T) {
	ops := []byte{'e', 'm', 'd', 'b', 'o', 'x', 'y', 'z'}
	for _, op := range ops {
		for mag := 1; mag <= 9; mag++ {
			original := string([]byte{op, byte('0' + mag)})
			encoded := EncodeBitOpsBinary([]byte(original))
			decoded, err := DecodeBitOpsBinary(encoded)
			if err != nil {
				t.Fatalf("%s: decode failed: %v", original, err)
			}
			if string(decoded) != original {
				t.Fatalf("%s%d round-trip: got %q, want %q",
					string(op), mag, string(decoded), original)
			}
		}
	}
}

func TestLongIndexRoundTrip(t *testing.T) {
	for _, length := range []uint64{255, 256, 1000, 100000} {
		tok := PacketToken{
			Kind:      PacketTokenIndexOperand,
			Magnitude: length,
			Variable:  0,
		}
		enc := EncodePacketOperandToken(tok)
		if len(enc) == 0 {
			t.Fatalf("length %d: encode returned nil", length)
		}
		r := bytes.NewReader(enc[1:])
		got, err := ReadPacketOperandToken(r, enc[0])
		if err != nil {
			t.Fatalf("length %d: decode: %v", length, err)
		}
		if got.Magnitude != length {
			t.Fatalf("length %d: round-trip got %d", length, got.Magnitude)
		}
	}
}

func TestExtendedMagnitudeRoundTrip(t *testing.T) {
	ops := []byte{'e', 'm', 'd', 'b', 'o', 'x', 'y', 'z'}
	for _, op := range ops {
		for mag := 10; mag <= 31; mag++ {
			tok := PacketToken{
				Kind:      PacketTokenMagnitudeOperand,
				Op:        op,
				Magnitude: uint64(mag),
			}
			enc := EncodePacketOperandToken(tok)
			if len(enc) < 2 || enc[0] != compactOperandExtended {
				t.Fatalf("%c%d: encode produced %v", op, mag, enc)
			}
			r := bytes.NewReader(enc[1:])
			got, err := ReadPacketOperandToken(r, compactOperandExtended)
			if err != nil {
				t.Fatalf("%c%d: decode: %v", op, mag, err)
			}
			if got.Op != op || got.Magnitude != uint64(mag) {
				t.Fatalf("%c%d: got %c%d", op, mag, got.Op, got.Magnitude)
			}
		}
	}
}

func TestTwoDigitMagnitudeParsing(t *testing.T) {
	cases := []struct {
		input string
		op    byte
		mag   uint64
		end   int
	}{
		{"e1-", 'e', 1, 2},
		{"e9-", 'e', 9, 2},
		{"e10-", 'e', 10, 3},
		{"e15-", 'e', 15, 3},
		{"e31-", 'e', 31, 3},
		{"m5,", 'm', 5, 2},
		{"m25;", 'm', 25, 3},
	}
	for _, c := range cases {
		tok, end, ok := ParseOperandTokenBytes([]byte(c.input), 0)
		if !ok {
			t.Fatalf("%q: parse failed", c.input)
		}
		if end != c.end {
			t.Fatalf("%q: consumed %d, want %d", c.input, end, c.end)
		}
		if tok.Op != c.op || tok.Magnitude != c.mag {
			t.Fatalf("%q: got %c%d", c.input, tok.Op, tok.Magnitude)
		}
	}
}
