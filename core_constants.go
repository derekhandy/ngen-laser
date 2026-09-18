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

type PacketTokenKind uint8
type PackageCodec byte

const (
	// GLOBAL SETTINGS
	seed            = 777
	analyticsHeader = "ID,GUID,Original Size,Packaged Size,Total Comp,[] C Norm.,Inst Avg (Time),Compute Time,Total Time" +
		",[] T Norm.,Population,Iteration Limit,Passes,Elite Percent,Mutated Elite Percent,New Network Percent" +
		",Input Size,Hidden Layers,Latent Size,Token Stride,Temperature,[] F Norm.,Best Network,Best Fitness,Instructions"

	// VERSION INFO
	bitOpsVersion byte = 5

	// CODEC PARAMS
	packetSize        = 27648
	maxStringLength   = 500000
	maxExpansionLimit = 5000

	coordinateLength = 3
	lineLength       = 6

	variableTokenCount   = 26
	variableMaxMagnitude = 200

	// CODEC EXT. FORMAT
	CodecRaw     PackageCodec = 0x00 // default, no compression
	CodecDeflate PackageCodec = 0x01 // compress/flate, DefaultCompression
	CodecZstd    PackageCodec = 0x02 // reserved for github.com/klauspost/compress/zstd
	CodecXz      PackageCodec = 0x03 // reserved for github.com/ulikunitz/xz

	// NETWORK SETTINGS
	maxNetworkSteps     = 20
	maxSavedElites      = 15
	iterationMultiplier = 1

	shapedFitnessTieBreakerScale = 1.0
	packetSizeFitnessScale       = 999998.0

	crossoverMutateChance = 0.9

	// SERIALIZATION
	PacketTokenBitRun PacketTokenKind = iota
	PacketTokenFixedOperand
	PacketTokenMagnitudeOperand
	PacketTokenDirectionalOperand
	PacketTokenAxisOperand
	PacketTokenIndexOperand
	PacketTokenVariableRef
	PacketTokenRawOperandRun

	// compactOperandMagCount = 8 operands * 9 magnitudes
	// compactOperandDirCount = 2 operands * 6 directions
	compactOperandFixedBase  byte = 0x80
	compactOperandFixedCount      = 5
	compactOperandMagBase    byte = compactOperandFixedBase + compactOperandFixedCount
	compactOperandMagCount        = 72
	compactOperandDirBase    byte = compactOperandMagBase + compactOperandMagCount
	compactOperandDirCount        = 12
	compactOperandRotBase    byte = compactOperandDirBase + compactOperandDirCount
	compactOperandRotCount        = 3
	compactOperandIndex      byte = compactOperandRotBase + compactOperandRotCount
	compactOperandIndexLong  byte = compactOperandIndex + 1
	compactOperandRawRun     byte = compactOperandIndexLong + 1
	compactOperandVarBase    byte = compactOperandRawRun + 1
	compactOperandIndexVar   byte = compactOperandVarBase + byte(variableTokenCount)

	compactBitRunMax = 128

	// SERIALIZATION EXT.

	// Extended operand prefix. When 0xFC appears as a token byte, the next
	// byte selects an entry in the extended table.
	compactOperandExtended byte = 0xFC

	// Extended table slot allocation.
	//   0x00 – 0x08 : s1..s9                                (9 slots)
	//   0x09 – 0xB8 : magnitudes 10–31 for e/m/d/b/o/x/y/z (176 slots)
	//   0xB9 – 0xFF : reserved                             (71 slots)
	extSwapBase  byte = 0x00
	extSwapMax   byte = extSwapBase + 9
	extMagHiBase byte = extSwapMax
	extMagHiMax  byte = extMagHiBase + 176
)

var (
	operandLineLength = 1
	operandLength     = lineLength * operandLineLength
	version           = "v3.0.0"
	packageFormat     = []byte("LASER-PACKAGE-v3.0.0\x00")
)

func UpdateOperandLength(length int) {
	operandLineLength = length
	operandLength = lineLength * operandLineLength
}
