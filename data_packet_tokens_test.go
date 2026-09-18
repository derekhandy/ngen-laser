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
