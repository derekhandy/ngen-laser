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

const stateHistoryLength = 8

type MemoryController struct {
	recurrentConfig RecurrentStateConfig
	attentionConfig AttentionConfig
	layerCount      int

	state    *RecurrentState
	history  [][]float32 // [stateHistoryLength][HiddenSize]
	histHead int         // next write slot
	histLen  int         // number of valid entries (0..len(history))

	encoder []AttentionLayer
	weights *GRUWeights
}

type MemoryConfig struct {
	rCfg       RecurrentStateConfig
	aCfg       AttentionConfig
	layerCount int
}

func NewMemoryController(m *MemoryConfig) *MemoryController {
	controller := &MemoryController{
		recurrentConfig: m.rCfg,
		attentionConfig: m.aCfg,
		layerCount:      m.layerCount,
		state:           NewRecurrentState(m.rCfg),
		encoder:         make([]AttentionLayer, m.layerCount),
	}

	if m.rCfg.UseGRU {
		controller.weights = InitializeGRUWeights(m.rCfg.HiddenSize)
	}

	for i := 0; i < m.layerCount; i++ {
		controller.encoder[i] = NewAttentionLayer(m.aCfg, m.rCfg.HiddenSize)
	}

	controller.history = make([][]float32, stateHistoryLength)
	for i := range controller.history {
		controller.history[i] = make([]float32, m.rCfg.HiddenSize)
	}
	return controller
}

func NewWorkingMemory(master *MemoryController) *MemoryController {
	if master == nil {
		return nil
	}

	localEncoder := make([]AttentionLayer, len(master.encoder))
	for i := range master.encoder {
		localEncoder[i] = CloneAttentionBuffers(master.encoder[i])
	}

	wm := &MemoryController{
		recurrentConfig: master.recurrentConfig,
		attentionConfig: master.attentionConfig,
		layerCount:      master.layerCount,
		state:           NewRecurrentState(master.recurrentConfig),
		encoder:         localEncoder,
		weights:         master.weights,
	}

	wm.history = make([][]float32, len(master.history))
	for i := range master.history {
		wm.history[i] = make([]float32, len(master.history[i]))
	}

	return wm
}

func (c *MemoryController) Reset() {
	if c == nil || c.state == nil {
		return
	}
	for i := range c.state.vector {
		c.state.vector[i] = 0
	}
	for i := range c.history {
		for j := range c.history[i] {
			c.history[i][j] = 0
		}
	}
	c.histHead = 0
	c.histLen = 0
}

func (c *MemoryController) pushHistory(vec []float32) {
	if len(c.history) == 0 {
		return
	}
	copy(c.history[c.histHead], vec)
	c.histHead = (c.histHead + 1) % len(c.history)
	if c.histLen < len(c.history) {
		c.histLen++
	}
}

func (c *MemoryController) ForwardHybrid(input []float32) []float32 {
	if c == nil {
		return input
	}

	output := UpdateRecurrentState(c.state, input, c.weights)
	if output == nil {
		return input
	}

	// Push the *new* state so attention can look back at prior steps.
	c.pushHistory(output)

	for i := range c.encoder {
		output = AttentionForward(&c.encoder[i], output, c.history, c.histLen, c.histHead)
	}
	return output
}

func CloneMemoryController(src *MemoryController) *MemoryController {
	if src == nil {
		return nil
	}

	clone := &MemoryController{
		recurrentConfig: src.recurrentConfig,
		attentionConfig: src.attentionConfig,
		layerCount:      src.layerCount,
		state:           NewRecurrentState(src.recurrentConfig),
		encoder:         make([]AttentionLayer, len(src.encoder)),
		weights:         src.weights,
	}

	copy(clone.state.vector, src.state.vector)
	for i := range src.encoder {
		clone.encoder[i] = CloneAttentionLayer(src.encoder[i])
	}

	clone.history = make([][]float32, len(src.history))
	for i := range src.history {
		clone.history[i] = append([]float32(nil), src.history[i]...)
	}
	clone.histHead = src.histHead
	clone.histLen = src.histLen

	return clone
}

func CloneAttentionLayer(src AttentionLayer) AttentionLayer {
	clone := src
	clone.qProj = CloneLayer(src.qProj)
	clone.kProj = CloneLayer(src.kProj)
	clone.vProj = CloneLayer(src.vProj)
	clone.oProj = CloneLayer(src.oProj)
	clone.ffn1 = CloneLayer(src.ffn1)
	clone.ffn2 = CloneLayer(src.ffn2)
	clone = CloneAttentionBuffers(clone)

	return clone
}

func CloneAttentionBuffers(src AttentionLayer) AttentionLayer {
	clone := src
	clone.qBuffer = append([]float32(nil), src.qBuffer...)
	clone.kBuffer = append([]float32(nil), src.kBuffer...)
	clone.vBuffer = append([]float32(nil), src.vBuffer...)
	clone.oBuffer = append([]float32(nil), src.oBuffer...)
	clone.ffn1Buffer = append([]float32(nil), src.ffn1Buffer...)
	clone.ffn2Buffer = append([]float32(nil), src.ffn2Buffer...)

	clone.scores = append([]float32(nil), src.scores...)
	clone.combined = append([]float32(nil), src.combined...)

	if len(src.kCache) > 0 {
		clone.kCache = make([][]float32, len(src.kCache))
		clone.vCache = make([][]float32, len(src.vCache))
		for i := range src.kCache {
			clone.kCache[i] = append([]float32(nil), src.kCache[i]...)
			clone.vCache[i] = append([]float32(nil), src.vCache[i]...)
		}
	}
	return clone
}

func DefaultMemoryConfig(hiddenSize int) *MemoryConfig {
	if hiddenSize <= 0 {
		hiddenSize = latentSize
	}
	rCfg := DefaultRecurrentStateConfig()
	rCfg.HiddenSize = hiddenSize
	return &MemoryConfig{
		rCfg:       rCfg,
		aCfg:       AttentionConfig{HiddenSize: hiddenSize, NumHeads: 8, UseLayerNorm: true},
		layerCount: 4,
	}
}

func NetworkLatentSize(net *Network) int {
	if net == nil {
		return latentSize
	}
	if len(net.backbone) > 0 {
		last := net.backbone[len(net.backbone)-1]
		if len(last.weights) > 0 {
			return len(last.weights)
		}
	}
	if len(net.heads.Index.weights) > 0 {
		return len(net.heads.Index.weights[0])
	}
	if len(defaultHiddenLayers) > 0 {
		return defaultHiddenLayers[len(defaultHiddenLayers)-1]
	}
	return latentSize
}

func NetworkInputSize(net *Network) int {
	if net != nil && net.inputSize > 0 {
		return net.inputSize
	}
	return defaultInputSize
}

func EnsureNetworkMemory(net *Network) *MemoryController {
	if net == nil {
		return nil
	}
	_latentSize := NetworkLatentSize(net)
	if net.memory == nil ||
		net.memory.state == nil ||
		len(net.memory.state.vector) != _latentSize ||
		net.memory.recurrentConfig.HiddenSize != _latentSize ||
		net.memory.attentionConfig.HiddenSize != _latentSize ||
		!memoryHistoryValid(net.memory, _latentSize) ||
		len(net.memory.encoder) == 0 {
		net.memory = NewMemoryController(DefaultMemoryConfig(_latentSize))
		return net.memory
	}
	if net.memory.layerCount <= 0 {
		net.memory.layerCount = len(net.memory.encoder)
	}

	return net.memory
}

func memoryHistoryValid(m *MemoryController, width int) bool {
	if m == nil || len(m.history) != stateHistoryLength {
		return false
	}
	if stateHistoryLength == 0 {
		return true
	}
	return len(m.history[0]) == width
}

func MutateMemory(mem *MemoryController, mutatePercent float32, mutationStrength float32) {
	if mem == nil {
		return
	}
	for i := range mem.encoder {
		MutateAttention(&mem.encoder[i], mutatePercent, mutationStrength)
	}
}

func MutateAttention(layer *AttentionLayer, mutatePercent float32, mutationStrength float32) {
	MutateLayer(&layer.qProj, mutatePercent, mutationStrength)
	MutateLayer(&layer.kProj, mutatePercent, mutationStrength)
	MutateLayer(&layer.vProj, mutatePercent, mutationStrength)
	MutateLayer(&layer.oProj, mutatePercent, mutationStrength)
	MutateLayer(&layer.ffn1, mutatePercent, mutationStrength)
	MutateLayer(&layer.ffn2, mutatePercent, mutationStrength)
}
