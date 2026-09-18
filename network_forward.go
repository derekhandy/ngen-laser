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
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
)

type IndexEntry struct {
	Lookup   string
	Variable string
}

func EncodeInstructionString(inputBuffer []float32, inst string, tokenLength int, coverage float32) []float32 {
	for i := range inputBuffer {
		inputBuffer[i] = 0
	}

	maxAllowedTokens := ComputeTokenLength(len(inputBuffer))
	actualTokenLimit := tokenLength
	if actualTokenLimit > maxAllowedTokens {
		actualTokenLimit = maxAllowedTokens
	}
	if actualTokenLimit < 0 {
		actualTokenLimit = 0
	}
	positionPeriod := float64(maxAllowedTokens)

	for i := 0; i < len(inst) && i < actualTokenLimit; i++ {
		offset := i * inputTokenStride

		if offset+inputTokenStride > len(inputBuffer) {
			break
		}
		switch inst[i] {
		case '-':
			inputBuffer[offset] = 1
		case ',':
			inputBuffer[offset+1] = 1
		case '.':
			inputBuffer[offset+2] = 1
		case ';':
			inputBuffer[offset+3] = 1
		}

		inputBuffer[offset+4] = float32(math.Sin(2.0*float64(math.Pi)*float64(i)/positionPeriod)) / float32(actualTokenLimit)
	}

	base := maxAllowedTokens * inputTokenStride

	if base+1 < len(inputBuffer) {
		inputBuffer[base] = 1
		inputBuffer[base+1] = coverage

		for i := 2; i < additionalFeatureSize && (base+i) < len(inputBuffer); i++ {
			inputBuffer[base+i] = 0
		}
	}

	return inputBuffer
}

func ForwardPass(net *Network, input []float32, memory *MemoryController) (NetworkOutput, error) {
	if net != nil && len(net.layerBuffers) == 0 && len(net.backbone) > 0 {
		AllocateNetworkBuffers(net)
	}
	if err := ValidateForwardState(net, input, memory); err != nil {
		return NewZeroedOutput(), err
	}

	activation := input
	for i, layer := range net.backbone {
		activation = LayerActivation(activation, layer, net.layerBuffers[i])
		if activation == nil {
			return NewZeroedOutput(), fmt.Errorf("backbone layer %d has invalid activation state", i)
		}
	}

	activation = memory.ForwardHybrid(activation)

	if activation == nil {
		return NewZeroedOutput(), errors.New("memory controller returned invalid activation state")
	}
	return RunOutputHeads(net.heads, activation), nil
}

func ValidateForwardState(net *Network, input []float32, memory *MemoryController) error {
	if net == nil {
		return errors.New("network is nil")
	}

	if memory == nil || memory.state == nil {
		return errors.New("memory controller or recurrent state is nil")
	}
	if len(memory.history) != stateHistoryLength {
		return fmt.Errorf("memory history length %d, want %d", len(memory.history), stateHistoryLength)
	}
	if len(memory.history) > 0 && len(memory.history[0]) != memory.recurrentConfig.HiddenSize {
		return fmt.Errorf("memory history slot width %d does not match backbone output %d",
			len(memory.history[0]), memory.recurrentConfig.HiddenSize)
	}
	if len(input) != net.inputSize {
		return fmt.Errorf("input size %d does not match network input size %d", len(input), net.inputSize)
	}
	if len(net.layerBuffers) != len(net.backbone) {
		return fmt.Errorf("network has %d layer buffers for %d backbone layers", len(net.layerBuffers), len(net.backbone))
	}

	previousSize := net.inputSize
	for i, layer := range net.backbone {
		if err := validateForwardLayer(layer, previousSize, len(net.layerBuffers[i])); err != nil {
			return fmt.Errorf("backbone layer %d: %w", i, err)
		}
		previousSize = len(layer.weights)
	}

	for name, layer := range map[string]Layer{
		"index":                 net.heads.Index,
		"operand":               net.heads.Operand,
		"translation direction": net.heads.TranslationDir,
		"translation magnitude": net.heads.TranslationMag,
		"rotation axis":         net.heads.RotationAxis,
		"scaling magnitude":     net.heads.ScalingMagnitude,
		"insertion length":      net.heads.InsertionLength,
		"stop":                  net.heads.Stop,
	} {
		if err := validateForwardLayer(layer, previousSize, 0); err != nil {
			return fmt.Errorf("%s head: %w", name, err)
		}
	}

	if len(memory.state.vector) != memory.recurrentConfig.HiddenSize {
		return fmt.Errorf("recurrent state size %d does not match configured size %d", len(memory.state.vector), memory.recurrentConfig.HiddenSize)
	}
	if len(memory.state.vector) != previousSize {
		return fmt.Errorf("recurrent state size %d does not match backbone output size %d", len(memory.state.vector), previousSize)
	}
	if memory.recurrentConfig.UseGRU && !ValidGRUWeights(memory.weights, memory.recurrentConfig.HiddenSize) {
		return errors.New("memory controller has invalid GRU weights")
	}
	return nil
}

func validateForwardLayer(layer Layer, inputSize, bufferSize int) error {
	if len(layer.weights) == 0 {
		return errors.New("layer has no weights")
	}
	if len(layer.biases) != len(layer.weights) {
		return fmt.Errorf("has %d biases for %d output rows", len(layer.biases), len(layer.weights))
	}
	if bufferSize != 0 && bufferSize != len(layer.weights) {
		return fmt.Errorf("has buffer size %d for %d outputs", bufferSize, len(layer.weights))
	}
	for i, row := range layer.weights {
		if len(row) != inputSize {
			return fmt.Errorf("row %d has width %d, expected %d", i, len(row), inputSize)
		}
	}
	return nil
}

func LayerActivation(input []float32, layer Layer, output []float32) []float32 {
	if len(output) != len(layer.weights) || len(layer.biases) != len(layer.weights) {
		return nil
	}

	for i, neuronWeights := range layer.weights {
		sum := layer.biases[i]

		for j, weight := range neuronWeights {
			if j < len(input) {
				sum += input[j] * weight
			}
		}

		switch layer.activation {
		case "sigmoid":
			output[i] = 0.5 * (sum/float32(1.0+math.Abs(float64(sum))) + 1.0)
		case "relu":
			if sum > 0 {
				output[i] = sum
			} else {
				output[i] = 0
			}
		case "tanh":
			output[i] = FastTanh(sum)
		case "linear":
			output[i] = sum
		default:
			if sum > 0 {
				output[i] = sum
			} else {
				output[i] = 0
			}
		}
	}

	return output
}

func BuildOperandString(current, operand string, params map[string]interface{}) string {
	switch operand {
	case "h", "k":
		return operand
	case "e":
		return operand + MagnitudeToDigits(OperandMagnitude(params))
	case "t", "f":
		dir, _ := params["direction"].(string)
		if dir == "" {
			return ""
		}
		return operand + dir
	case "x", "y", "z":
		return operand + MagnitudeToDigits(OperandMagnitude(params))
	case "r":
		axis, _ := params["axis"].(string)
		if axis == "" {
			return ""
		}
		return operand + axis
	case "m", "d", "b", "o":
		return operand + MagnitudeToDigits(OperandMagnitude(params))
	case "n", "g", "l":
		return operand
	case "i":
		lengthVal, _ := params["length"].(int)
		lengthVal = ClampIndexLength(lengthVal, len(current))
		variable := DeterminePreviousVariable(current)
		return operand + strconv.Itoa(lengthVal) + variable
	default:
		return ""
	}
}

func ApplyOperation(rollout *Rollout, current string, idx int, operand string, params map[string]interface{}) (string, error) {
	if rollout != nil && !rollout.OperationAllowed(operand) {
		return "", errors.New("operand disabled by training flags")
	}

	params = CloneParams(params)
	if operand == "i" {
		lengthVal, _ := params["length"].(int)
		params["length"] = ClampIndexLength(lengthVal, len(current))
	}

	if idx < 0 {
		idx = 0
	}
	if idx > len(current) {
		idx = len(current)
	}

	minIdx, maxIdx := ComputeInsertionIndexRange(len(current), operand, params)
	if idx < minIdx {
		idx = minIdx
	}
	if idx > maxIdx {
		idx = maxIdx
	}

	insertion := BuildOperandString(current, operand, params)
	if insertion == "" {
		return "", errors.New("unsupported operand")
	}

	return current[:idx] + "!" + insertion + current[idx:], nil
}

func MinHeadForOperand(operand string, params map[string]interface{}) int {
	switch operand {
	case "h", "k":
		return 1
	case "e", "b", "o":
		return OperandMagnitude(params)
	case "t", "f", "x", "y", "z", "r", "m", "d", "n", "g", "l":
		return operandDigitLength()
	default:
		return 0
	}
}

func MinTailForOperand(operand string, params map[string]interface{}) int {
	switch operand {
	case "h":
		return 2
	case "k":
		return 3
	case "e", "b", "o":
		return OperandMagnitude(params)
	case "t", "f", "x", "y", "z", "r", "m", "d", "n", "g", "l":
		return operandDigitLength()
	case "i":
		lengthVal, _ := params["length"].(int)
		return MaxInt(1, lengthVal)
	default:
		return 0
	}
}

func OperandMagnitude(params map[string]interface{}) int {
	if params == nil {
		return 1
	}
	val, ok := params["magnitude"].(int)
	if !ok {
		return 1
	}
	return ClampInt(val, 1, 9)
}

func CloneParams(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	target := make(map[string]interface{}, len(src))
	for k, v := range src {
		target[k] = v
	}
	return target
}

func (metrics StepMetrics) PacketSizeReduction() int {
	if metrics.previousSize > 0 && metrics.newSize > 0 {
		return metrics.previousSize - metrics.newSize
	}
	if metrics.originalSize > 0 && metrics.newSize > 0 {
		return metrics.originalSize - metrics.newSize
	}
	return metrics.sizeReduction
}

func DecodeDecision(output NetworkOutput, length int, r *rand.Rand, rollout *Rollout) (int, string, map[string]interface{}) {
	raw := FlattenNetworkOutput(output)
	idx, operand, params := DecodeNetworkDecision(raw, length, r, rollout)
	return idx, operand, params
}

func MagnitudeToDigits(magnitude int) string {
	if magnitude < 1 {
		magnitude = 1
	}
	return strconv.Itoa(magnitude)
}

func MaxIndexLengthForInstruction(instructionLength int) int {
	if instructionLength <= 1 {
		return 1
	}
	return MaxInt(1, instructionLength/2)
}

func ClampIndexLength(length, instructionLength int) int {
	return ClampInt(length, 1, MaxIndexLengthForInstruction(instructionLength))
}

func TokenFromIndex(index int) string {
	index = ClampInt(index, 0, variableTokenCount-1)
	return string(rune('A' + index))
}
