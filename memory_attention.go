package main

import (
	"math"
)

type AttentionConfig struct {
	HiddenSize   int
	NumHeads     int
	UseLayerNorm bool
}

type AttentionLayer struct {
	config AttentionConfig

	qProj Layer
	kProj Layer
	vProj Layer
	oProj Layer

	ffn1 Layer
	ffn2 Layer

	qBuffer    []float32
	kBuffer    []float32
	vBuffer    []float32
	oBuffer    []float32
	ffn1Buffer []float32
	ffn2Buffer []float32

	// scratch, all preallocated
	kCache   [][]float32 // [stateHistoryLength][HiddenSize]
	vCache   [][]float32 // [stateHistoryLength][HiddenSize]
	scores   []float32   // [stateHistoryLength]
	combined []float32   // [HiddenSize]
}

func AllocateAttentionBuffers(layer *AttentionLayer) {
	layer.qBuffer = make([]float32, len(layer.qProj.weights))
	layer.kBuffer = make([]float32, len(layer.kProj.weights))
	layer.vBuffer = make([]float32, len(layer.vProj.weights))
	layer.oBuffer = make([]float32, len(layer.oProj.weights))
	layer.ffn1Buffer = make([]float32, len(layer.ffn1.weights))
	layer.ffn2Buffer = make([]float32, len(layer.ffn2.weights))

	hidden := layer.config.HiddenSize
	if hidden <= 0 {
		hidden = latentSize
	}
	layer.kCache = make([][]float32, stateHistoryLength)
	layer.vCache = make([][]float32, stateHistoryLength)
	for i := range layer.kCache {
		layer.kCache[i] = make([]float32, hidden)
		layer.vCache[i] = make([]float32, hidden)
	}
	layer.scores = make([]float32, stateHistoryLength)
	layer.combined = make([]float32, hidden)
}

func EnsureHeads(config AttentionConfig) AttentionConfig {
	if config.NumHeads <= 0 {
		config.NumHeads = 8
	}
	if config.HiddenSize <= 0 {
		config.HiddenSize = latentSize
	}
	return config
}

func NewAttentionLayer(config AttentionConfig, inputSize int) AttentionLayer {
	config = EnsureHeads(config)
	if inputSize <= 0 {
		inputSize = config.HiddenSize
	}

	layer := AttentionLayer{
		config: config,
		qProj:  InitializeLayer(inputSize, config.HiddenSize, "linear"),
		kProj:  InitializeLayer(inputSize, config.HiddenSize, "linear"),
		vProj:  InitializeLayer(inputSize, config.HiddenSize, "linear"),
		oProj:  InitializeLayer(config.HiddenSize, config.HiddenSize, "linear"),
		ffn1:   InitializeLayer(config.HiddenSize, config.HiddenSize, "relu"),
		ffn2:   InitializeLayer(config.HiddenSize, config.HiddenSize, "linear"),
	}
	AllocateAttentionBuffers(&layer)
	return layer
}

func LayerNormInPlace(vec []float32) {
	if len(vec) == 0 {
		return
	}
	sum := float32(0.0)
	for _, v := range vec {
		sum += v
	}
	mean := sum / float32(len(vec))

	variance := float32(0.0)
	for _, v := range vec {
		d := v - mean
		variance += d * d
	}
	variance /= float32(len(vec))

	denom := math.Sqrt(float64(variance + 1e-8))
	for i := range vec {
		vec[i] = (vec[i] - mean) / float32(denom)
	}
}

// AttentionForward performs single-query, multi-key attention over the
// ring buffer of past recurrent states. For each history entry it computes
// K and V, scores the current query against every entry, softmaxes the
// scores, and returns a weighted sum of V, then runs the output projection,
// residual, LayerNorm, and FFN.
func AttentionForward(
	layer *AttentionLayer,
	query []float32,
	history [][]float32,
	histLen int,
	histHead int,
) []float32 {
	if layer == nil || len(query) == 0 {
		return query
	}
	config := EnsureHeads(layer.config)
	if len(layer.qBuffer) == 0 || len(layer.kCache) != stateHistoryLength {
		AllocateAttentionBuffers(layer)
	}

	residual := append([]float32(nil), query...)
	q := LayerActivation(query, layer.qProj, layer.qBuffer)
	if q == nil {
		return query
	}

	dim := config.HiddenSize
	numHeads := config.NumHeads
	headDim := dim / numHeads
	if headDim <= 0 {
		numHeads = 1
		headDim = dim
	}

	// Zero the combined buffer.
	for i := range layer.combined {
		layer.combined[i] = 0
	}

	// Precompute K and V for the valid history entries.
	valid := histLen
	if valid > len(history) {
		valid = len(history)
	}
	if valid > stateHistoryLength {
		valid = stateHistoryLength
	}

	for h := 0; h < valid; h++ {
		// Oldest first.
		idx := (histHead - valid + h + len(history)) % len(history)
		past := history[idx]

		k := LayerActivation(past, layer.kProj, layer.kBuffer)
		v := LayerActivation(past, layer.vProj, layer.vBuffer)
		if k == nil || v == nil {
			return query
		}
		copy(layer.kCache[h], k)
		copy(layer.vCache[h], v)
	}

	scale := float32(1.0)
	if headDim > 0 {
		scale = float32(1.0 / math.Sqrt(float64(headDim)))
	}

	for head := 0; head < numHeads; head++ {
		start := head * headDim
		end := start + headDim
		if start >= dim {
			break
		}
		if end > dim {
			end = dim
		}

		if valid == 0 {
			// Degenerate: copy the query slice into the output.
			copy(layer.combined[start:end], q[start:end])
			continue
		}

		// Score query against every history key.
		maxScore := float32(math.Inf(-1))
		for h := 0; h < valid; h++ {
			k := layer.kCache[h]
			score := float32(0)
			for i := start; i < end; i++ {
				score += q[i] * k[i]
			}
			score *= scale
			layer.scores[h] = score
			if score > maxScore {
				maxScore = score
			}
		}

		// Softmax over history.
		sum := float32(0)
		for h := 0; h < valid; h++ {
			e := float32(math.Exp(float64(layer.scores[h] - maxScore)))
			layer.scores[h] = e
			sum += e
		}
		if sum == 0 {
			sum = 1
		}
		invSum := float32(1.0) / sum

		// Weighted sum of values.
		for h := 0; h < valid; h++ {
			w := layer.scores[h] * invSum
			v := layer.vCache[h]
			for i := start; i < end; i++ {
				layer.combined[i] += w * v[i]
			}
		}
	}

	attnOut := LayerActivation(layer.combined, layer.oProj, layer.oBuffer)
	if attnOut == nil {
		return query
	}

	if len(attnOut) == len(residual) {
		for i := range attnOut {
			attnOut[i] += residual[i]
		}
	}

	if config.UseLayerNorm {
		LayerNormInPlace(attnOut)
	}

	ff1 := LayerActivation(attnOut, layer.ffn1, layer.ffn1Buffer)
	if ff1 == nil {
		return attnOut
	}
	return LayerActivation(ff1, layer.ffn2, layer.ffn2Buffer)
}
