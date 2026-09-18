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
	"math"
	"math/rand"

	"github.com/google/uuid"
)

var (
	operandClasses = []string{
		"h", "k", "e", "o",
		"t", "f",
		"x", "y", "z",
		"r", "m", "d", "n",
		"i",
		"b", "g", "l",
	}
	directionClasses = []string{"u", "d", "l", "r", "f", "b"}
	axisClasses      = []string{"x", "y", "z"}

	defaultHiddenLayers   = []int{512, 512, 256, 128}
	scalarFeatureCount    = 12
	additionalFeatureSize = scalarFeatureCount + len(operandClasses)
	defaultInputSize      = 490
	inputTokenStride      = 5
	defaultTokenLength    = ComputeTokenLength(defaultInputSize)
	latentSize            = 128
)

func IsStringOperation(operand string) bool {
	switch operand {
	case "h", "k", "e", "o", "i", "b", "g", "l":
		return true
	default:
		return false
	}
}

func IsMathOperation(operand string) bool {
	switch operand {
	case "t", "f", "x", "y", "z", "r", "m", "d", "n":
		return true
	default:
		return false
	}
}

func SetDefaultArchitecture(inputSize int, hiddenLayers []int) {
	if inputSize > 0 {
		defaultInputSize = inputSize
		defaultTokenLength = ComputeTokenLength(defaultInputSize)
	}
	if len(hiddenLayers) > 0 {
		defaultHiddenLayers = append([]int(nil), hiddenLayers...)
	}
}

func ComputeTokenLength(inputSize int) int {
	available := inputSize - additionalFeatureSize
	if available <= 0 {
		return 0
	}
	return available / inputTokenStride
}

func ContextWindowForLength(length int) int {
	window := length
	if window > defaultTokenLength {
		window = defaultTokenLength
	}
	if window < 24 {
		window = 24
	}
	return window
}

type Layer struct {
	weights    [][]float32
	biases     []float32
	activation string
}

type OutputHeads struct {
	Index            Layer
	Operand          Layer
	TranslationDir   Layer
	TranslationMag   Layer
	RotationAxis     Layer
	ScalingMagnitude Layer
	InsertionLength  Layer
	Stop             Layer
}

type Network struct {
	networkName string
	inputSize   int
	backbone    []Layer
	heads       OutputHeads
	memory      *MemoryController

	layerBuffers [][]float32
}

func AllocateNetworkBuffers(net *Network) {
	net.layerBuffers = make([][]float32, len(net.backbone))
	for i, layer := range net.backbone {
		net.layerBuffers[i] = make([]float32, len(layer.weights))
	}
}

func InitializeNetwork(inputSize int, hiddenLayers []int, outputSize int, i int) *Network {
	network := Network{
		networkName: uuid.New().String(),
		inputSize:   inputSize,
		backbone:    make([]Layer, 0),
	}

	prevSize := inputSize
	for idx, layerSize := range hiddenLayers {
		activation := "relu"
		if idx == len(hiddenLayers)-1 {
			activation = "tanh"
		}
		network.backbone = append(network.backbone, InitializeLayer(prevSize, layerSize, activation))
		prevSize = layerSize
	}

	commonSize := prevSize
	network.heads = OutputHeads{
		Index:            InitializeLayer(commonSize, 1, "linear"),
		Operand:          InitializeLayer(commonSize, len(operandClasses), "linear"),
		TranslationDir:   InitializeLayer(commonSize, len(directionClasses), "linear"),
		TranslationMag:   InitializeLayer(commonSize, 1, "linear"),
		RotationAxis:     InitializeLayer(commonSize, len(axisClasses), "linear"),
		ScalingMagnitude: InitializeLayer(commonSize, 1, "linear"),
		InsertionLength:  InitializeLayer(commonSize, 1, "linear"),
		Stop:             InitializeLayer(commonSize, 1, "linear"),
	}

	network.memory = NewMemoryController(DefaultMemoryConfig(commonSize))

	return &network
}

func InitializeLayer(inputSize, outputSize int, activation string) Layer {
	weights := make([][]float32, outputSize)
	biases := make([]float32, outputSize)

	for i := range weights {
		weights[i] = make([]float32, inputSize)
		for j := range weights[i] {
			switch activation {
			case "relu":
				weights[i][j] = float32(rand.NormFloat64() * math.Sqrt(2.0/float64(inputSize)))
			case "tanh":
				weights[i][j] = float32(rand.NormFloat64() * math.Sqrt(1.0/float64(inputSize)))
			case "linear":
				weights[i][j] = float32(rand.NormFloat64() * 0.1)
			default:
				weights[i][j] = float32(rand.NormFloat64() * math.Sqrt(1.0/float64(inputSize)))
			}
		}

		if activation == "linear" && i == 0 {
			biases[i] = 0.5 + (rand.Float32()-0.5)*0.1
		} else {
			biases[i] = rand.Float32() * 0.1
		}
	}

	return Layer{
		weights:    weights,
		biases:     biases,
		activation: activation,
	}
}

type NetworkOutput struct {
	Index struct {
		Scalar float32
	}
	Operand struct {
		Logits []float32
	}
	Translation struct {
		DirectionLogits []float32
		Magnitude       float32
	}
	Rotation struct {
		AxisLogits []float32
	}
	Scaling struct {
		Magnitude float32
	}
	Insertion struct {
		Length float32
	}
	Stop struct {
		Score float32
	}
}

func TotalOutputSize() int {
	return 1 + len(operandClasses) + 1 + len(directionClasses) + len(axisClasses) + 1 + 1 + 1
}

func NewZeroedOutput() NetworkOutput {
	return NetworkOutput{
		Operand: struct{ Logits []float32 }{Logits: make([]float32, len(operandClasses))},
		Translation: struct {
			DirectionLogits []float32
			Magnitude       float32
		}{DirectionLogits: make([]float32, len(directionClasses))},
		Rotation: struct{ AxisLogits []float32 }{AxisLogits: make([]float32, len(axisClasses))},
	}
}

func RunOutputHeads(heads OutputHeads, activation []float32) NetworkOutput {
	out := NewZeroedOutput()

	out.Index.Scalar = RunHeadScalar(heads.Index, activation)
	RunHeadVector(heads.Operand, activation, out.Operand.Logits)
	out.Translation.Magnitude = RunHeadScalar(heads.TranslationMag, activation)
	RunHeadVector(heads.TranslationDir, activation, out.Translation.DirectionLogits)
	RunHeadVector(heads.RotationAxis, activation, out.Rotation.AxisLogits)
	out.Scaling.Magnitude = RunHeadScalar(heads.ScalingMagnitude, activation)
	out.Insertion.Length = RunHeadScalar(heads.InsertionLength, activation)
	out.Stop.Score = RunHeadScalar(heads.Stop, activation)

	return out
}

func RunHeadScalar(layer Layer, input []float32) float32 {
	if len(layer.weights) == 0 {
		return 0
	}
	vector := RunHeadLinear(layer, input)
	return vector[0]
}

func RunHeadVector(layer Layer, input []float32, dst []float32) {
	if len(layer.weights) == 0 || len(dst) == 0 {
		return
	}
	values := RunHeadLinear(layer, input)
	copy(dst, values)
}

func RunHeadLinear(layer Layer, input []float32) []float32 {
	output := make([]float32, len(layer.weights))
	for i, weights := range layer.weights {
		sum := layer.biases[i]
		for j, w := range weights {
			if j < len(input) {
				sum += input[j] * w
			}
		}
		switch layer.activation {
		case "relu":
			output[i] = Relu(sum)
		case "tanh":
			output[i] = Tanh(sum)
		case "sigmoid":
			output[i] = Sigmoid(sum)
		default:
			output[i] = sum
		}
	}
	return output
}

func SliceNetworkOutput(flat []float32) NetworkOutput {
	if len(flat) != TotalOutputSize() {
		return NewZeroedOutput()
	}

	cursor := 0
	next := func() float32 {
		val := flat[cursor]
		cursor++
		return val
	}
	take := func(buf []float32) {
		copy(buf, flat[cursor:cursor+len(buf)])
		cursor += len(buf)
	}

	out := NewZeroedOutput()
	out.Index.Scalar = next()
	take(out.Operand.Logits)
	out.Translation.Magnitude = next()
	take(out.Translation.DirectionLogits)
	take(out.Rotation.AxisLogits)
	out.Scaling.Magnitude = next()
	out.Insertion.Length = next()
	out.Stop.Score = next()

	return out
}

func ApplyOutputConstraints(flat NetworkOutput) NetworkOutput {
	out := flat

	SoftmaxInPlace(out.Operand.Logits)
	SoftmaxInPlace(out.Translation.DirectionLogits)
	SoftmaxInPlace(out.Rotation.AxisLogits)

	out.Index.Scalar = Clamp(Sigmoid(out.Index.Scalar), 0.01, 0.99)
	out.Translation.Magnitude = Clamp(float32((math.Tanh(float64(out.Translation.Magnitude))+1)/2), 0, 1)
	out.Scaling.Magnitude = Clamp(float32((math.Tanh(float64(out.Scaling.Magnitude))+1)/2), 0, 1)
	out.Insertion.Length = Clamp(float32((math.Tanh(float64(out.Insertion.Length))+1)/2), 0, 1)
	out.Stop.Score = Sigmoid(out.Stop.Score)

	return out
}

func SoftmaxInPlace(values []float32) {
	if len(values) == 0 {
		return
	}

	maxVal := values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}

	sum := float32(0.0)
	for i, v := range values {
		exp := float32(math.Exp(float64(v - maxVal)))
		values[i] = exp
		sum += exp
	}

	if sum == float32(0) {
		uniform := 1.0 / float32(len(values))
		for i := range values {
			values[i] = uniform
		}
		return
	}

	for i := range values {
		values[i] /= sum
	}
}

func Argmax(probs []float32) int {
	bestIdx := 0
	maxVal := probs[0]
	for i, v := range probs {
		if v > maxVal {
			maxVal = v
			bestIdx = i
		}
	}
	return bestIdx
}

func SampleWithTemperature(values []float32, temperature float32, r *rand.Rand) int {
	if len(values) == 0 {
		return 0
	}
	if temperature <= 0 || r == nil {
		return Argmax(values)
	}

	maxVal := values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}

	sum := float32(0.0)
	probs := make([]float32, len(values))
	for i, v := range values {
		exp := float32(math.Exp(float64((v - maxVal) / temperature)))
		probs[i] = exp
		sum += exp
	}

	threshold := r.Float32() * sum
	acc := float32(0.0)
	for i, p := range probs {
		acc += p
		if threshold <= acc {
			return i
		}
	}

	return len(values) - 1
}

func BuildIndexLogits(scalar float32, slotCount int) []float32 {
	count := MaxInt(1, slotCount)
	idxLogits := make([]float32, count)
	scaled := scalar * float32(count-1)
	floorIdx := int(math.Floor(float64(scaled)))
	fraction := scaled - float32(floorIdx)

	for i := range idxLogits {
		distance := float32(math.Abs(float64(float32(i) - scaled)))
		idxLogits[i] = -distance
	}

	if floorIdx >= 0 && floorIdx < count {
		idxLogits[floorIdx] += 1 - fraction
	}
	if floorIdx+1 >= 0 && floorIdx+1 < count {
		idxLogits[floorIdx+1] += fraction
	}

	return idxLogits
}

func DecodeNetworkDecision(raw []float32, instructionLength int, r *rand.Rand, rollout *Rollout) (int, string, map[string]interface{}) {
	structured := ApplyOutputConstraints(SliceNetworkOutput(raw))
	FilterOperandLogitsByTrainingFlags(rollout, structured.Operand.Logits)

	if !AnyOperandAllowedByTrainingFlags(rollout) {
		return 0, "", map[string]interface{}{"stop": true}
	}

	operandIdx := Argmax(structured.Operand.Logits)
	if temp := rollout.DynamicTemperature(); temp > 0 {
		operandIdx = SampleWithTemperature(structured.Operand.Logits, temp, r)
	}
	operand := operandClasses[operandIdx]

	params := make(map[string]interface{})

	switch operand {
	case "t", "f":
		directionIdx := Argmax(structured.Translation.DirectionLogits)
		if temp := rollout.DynamicTemperature(); temp > 0 {
			directionIdx = SampleWithTemperature(structured.Translation.DirectionLogits, temp, r)
		}
		params["direction"] = directionClasses[directionIdx]
		params["magnitude"] = ToMagnitude(structured.Translation.Magnitude)
	case "x", "y", "z":
		axisIdx := Argmax(structured.Rotation.AxisLogits)
		if temp := rollout.DynamicTemperature(); temp > 0 {
			axisIdx = SampleWithTemperature(structured.Rotation.AxisLogits, temp, r)
		}
		params["axis"] = axisClasses[axisIdx]
		params["magnitude"] = ToMagnitude(structured.Scaling.Magnitude)
	case "r":
		axisIdx := Argmax(structured.Rotation.AxisLogits)
		if temp := rollout.DynamicTemperature(); temp > 0 {
			axisIdx = SampleWithTemperature(structured.Rotation.AxisLogits, temp, r)
		}
		params["axis"] = axisClasses[axisIdx]
	case "m", "d", "e", "b", "o":
		params["magnitude"] = ToMagnitude(structured.Scaling.Magnitude)
	case "i":
		params["length"] = ToIndexLength(structured.Insertion.Length, MaxIndexLengthForInstruction(instructionLength))
	}

	minIdx, maxIdx := ComputeInsertionIndexRange(instructionLength, operand, params)
	slots := maxIdx - minIdx + 1
	if slots < 1 {
		slots = 1
	}

	var idx int
	if slots == 1 {
		idx = minIdx
	} else {
		raw := structured.Index.Scalar
		if raw < 0 {
			raw = 0
		} else if raw > 1 {
			raw = 1
		}
		if temp := rollout.DynamicTemperature(); temp > 0 {
			idxLogits := BuildIndexLogits(raw, slots)
			idx = SampleWithTemperature(idxLogits, temp, r)
		} else {
			scale := float32(slots - 1)
			if scale <= 0 {
				idx = 0
			} else {
				idx = int(raw * scale)
			}
		}
		if idx < 0 {
			idx = 0
		}
		if idx >= slots {
			idx = slots - 1
		}
		idx += minIdx
	}

	stopProbability := structured.Stop.Score
	if r.Float32() < stopProbability && rollout.Settings.Config.AllowStopping {
		params["stop"] = true
	} else {
		params["stop"] = false
	}

	return idx, operand, params
}

func ToMagnitude(value float32) int {
	return ClampInt(int(value*8+1.5), 1, 9)
}

func ToIndexLength(value float32, maxLength int) int {
	length := int(value*float32(maxLength-1) + 1)
	if length < 1 {
		return 1
	}
	if maxLength > 0 && length > maxLength {
		return maxLength
	}
	return length
}

func ComputeInsertionIndexRange(instructionLength int, operand string, params map[string]interface{}) (int, int) {
	if instructionLength < 0 {
		instructionLength = 0
	}
	minIdx := MinHeadForOperand(operand, params)
	if minIdx > instructionLength {
		minIdx = instructionLength
	}
	maxIdx := instructionLength - MinTailForOperand(operand, params)
	if maxIdx < 0 {
		maxIdx = 0
	}
	if maxIdx > instructionLength {
		maxIdx = instructionLength
	}
	if maxIdx < minIdx {
		maxIdx = minIdx
	}
	return minIdx, maxIdx
}

func FlattenNetworkOutput(out NetworkOutput) []float32 {
	vec := make([]float32, 0, TotalOutputSize())
	vec = append(vec, out.Index.Scalar)
	vec = append(vec, out.Operand.Logits...)
	vec = append(vec, out.Translation.Magnitude)
	vec = append(vec, out.Translation.DirectionLogits...)
	vec = append(vec, out.Rotation.AxisLogits...)
	vec = append(vec, out.Scaling.Magnitude)
	vec = append(vec, out.Insertion.Length)
	vec = append(vec, out.Stop.Score)
	return vec
}
