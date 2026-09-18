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

import "github.com/klauspost/compress/zstd"

var zstdEncoder *zstd.Encoder

func init() {
	enc, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderConcurrency(1),
		zstd.WithWindowSize(1<<20),
	)
	if err != nil {
		panic(err)
	}
	zstdEncoder = enc
}

func DeflateBitOps(bitOps string) int {
	input := InstructionsToBytesRuntime(string(bitOps))
	if len(input) == 0 {
		return 0
	}
	return len(zstdEncoder.EncodeAll(input, nil))
}
