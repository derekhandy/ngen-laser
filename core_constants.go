package main

type PacketTokenKind uint8

const (
	seed = 777

	packetSize          = 27648
	maxStringLength     = 500000
	maxExpansionLimit   = 5000
	maxNetworkSteps     = 20
	maxSavedElites      = 15
	iterationMultiplier = 1

	coordinateLength = 3
	lineLength       = 6

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

	compactOperandFixedBase byte = 0x80
	compactOperandMagBase   byte = 0x85
	compactOperandDirBase   byte = 0xC4
	compactOperandRotBase   byte = 0xD0
	compactOperandIndex     byte = 0xD3
	compactOperandIndexLong byte = 0xD4
	compactOperandRawRun    byte = 0xD5
	compactOperandVarBase   byte = 0xD6
	compactOperandIndexVar  byte = 0xF0
	compactBitRunMax             = 128

	crossoverMutateChance = 0.9

	analyticsHeader = "ID,GUID,Original Size,Packaged Size,Total Comp,[] C Norm.,Inst Avg (Time),Compute Time,Total Time" +
		",[] T Norm.,Population,Iteration Limit,Passes,Elite Percent,Mutated Elite Percent,New Network Percent" +
		",Input Size,Hidden Layers,Latent Size,Token Stride,Temperature,[] F Norm.,Best Network,Best Fitness,Instructions"
)

var (
	operandLineLength = 2
	operandLength     = lineLength * operandLineLength
	version           = "v1.0.2"
)

func UpdateOperandLength(length int) {
	if length < 2 {
		panic("operandLineLength must be at least 2")
	}

	operandLineLength = length
	operandLength = lineLength * operandLineLength
}
