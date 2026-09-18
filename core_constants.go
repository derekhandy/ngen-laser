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

const (
	seed = 777

	bitOpsVersion byte = 4

	packetSize          = 27648
	maxStringLength     = 500000
	maxExpansionLimit   = 5000
	maxNetworkSteps     = 20
	maxSavedElites      = 15
	iterationMultiplier = 1

	coordinateLength = 3
	lineLength       = 6

	// NOTE: variableTokenCount + compactOperandVarBase must stay <= 0xFF.
	// Currently 0xDF + 26 = 0xF9, leaving 6 bytes of headroom.
	variableTokenCount   = 26
	variableMaxMagnitude = 200

	shapedFitnessTieBreakerScale = 1.0
	packetSizeFitnessScale       = 999998.0

	PacketTokenBitRun PacketTokenKind = iota
	PacketTokenFixedOperand
	PacketTokenMagnitudeOperand
	PacketTokenDirectionalOperand
	PacketTokenAxisOperand
	PacketTokenIndexOperand
	PacketTokenVariableRef
	PacketTokenRawOperandRun

	compactOperandFixedBase  byte = 0x80
	compactOperandFixedCount      = 5 // h k n g l
	compactOperandMagBase    byte = compactOperandFixedBase + compactOperandFixedCount
	compactOperandMagCount        = 72 // 8 ops × 9 magnitudes
	compactOperandDirBase    byte = compactOperandMagBase + compactOperandMagCount
	compactOperandDirCount        = 12 // 6 directions × 2 ops
	compactOperandRotBase    byte = compactOperandDirBase + compactOperandDirCount
	compactOperandRotCount        = 3
	compactOperandIndex      byte = compactOperandRotBase + compactOperandRotCount
	compactOperandIndexLong  byte = compactOperandIndex + 1
	compactOperandRawRun     byte = compactOperandIndexLong + 1
	compactOperandVarBase    byte = compactOperandRawRun + 1
	compactOperandIndexVar   byte = compactOperandVarBase + byte(variableTokenCount)
	compactBitRunMax              = 128

	crossoverMutateChance = 0.9

	analyticsHeader = "ID,GUID,Original Size,Packaged Size,Total Comp,[] C Norm.,Inst Avg (Time),Compute Time,Total Time" +
		",[] T Norm.,Population,Iteration Limit,Passes,Elite Percent,Mutated Elite Percent,New Network Percent" +
		",Input Size,Hidden Layers,Latent Size,Token Stride,Temperature,[] F Norm.,Best Network,Best Fitness,Instructions"
)

var (
	operandLineLength = 2
	operandLength     = lineLength * operandLineLength
	version           = "v2.0.0"
	packageFormat     = []byte("LASER-PACKAGE-v2.0.0\x00")
)

func UpdateOperandLength(length int) {
	if length < 2 {
		panic("operandLineLength must be at least 2")
	}

	operandLineLength = length
	operandLength = lineLength * operandLineLength
}
