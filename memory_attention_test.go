package main

import (
	"math"
	"testing"
)

// identityLayer returns a Layer whose weights form the identity matrix and
// whose biases are zero. Used to build deterministic test projections.
func identityLayer(dim int) Layer {
	weights := make([][]float32, dim)
	for i := range weights {
		weights[i] = make([]float32, dim)
		weights[i][i] = 1
	}
	return Layer{
		weights:    weights,
		biases:     make([]float32, dim),
		activation: "linear",
	}
}

// reluIdentityLayer is an identity projection with a ReLU activation.
func reluIdentityLayer(dim int) Layer {
	l := identityLayer(dim)
	l.activation = "relu"
	return l
}

// buildTestAttentionLayer constructs a single-head attention layer whose
// projections are all identity, so the forward pass reduces to
//
//	score_i  = (q · h_i) / sqrt(dim)
//	weight_i = softmax(score)_i
//	combined = Σ w_i · h_i
//	output   = ReLU(combined + q)
//
// LayerNorm is disabled to keep the math exact.
func buildTestAttentionLayer(dim int) AttentionLayer {
	layer := AttentionLayer{
		config: AttentionConfig{
			HiddenSize:   dim,
			NumHeads:     1,
			UseLayerNorm: false,
		},
		qProj: identityLayer(dim),
		kProj: identityLayer(dim),
		vProj: identityLayer(dim),
		oProj: identityLayer(dim),
		ffn1:  reluIdentityLayer(dim),
		ffn2:  identityLayer(dim),
	}
	AllocateAttentionBuffers(&layer)
	return layer
}

// TestAttentionForwardPrefersMatchingHistoryEntry verifies that when the
// query equals one of the history keys, that entry receives the largest
// softmax weight.
func TestAttentionForwardPrefersMatchingHistoryEntry(t *testing.T) {
	dim := 4
	layer := buildTestAttentionLayer(dim)

	history := make([][]float32, stateHistoryLength)
	for i := range history {
		history[i] = make([]float32, dim)
	}
	history[0][0] = 1 // oldest
	history[1][1] = 1
	history[2][2] = 1

	query := []float32{1, 0, 0, 0}
	out := AttentionForward(&layer, query, history, 3, 3)

	if len(out) != dim {
		t.Fatalf("expected output length %d, got %d", dim, len(out))
	}

	// Component 0 corresponds to the matching key and should dominate.
	if out[0] <= out[1] {
		t.Fatalf("expected component 0 (%f) > component 1 (%f)", out[0], out[1])
	}
	// Components 1 and 2 are symmetric, so weights and values are equal.
	if math.Abs(float64(out[1]-out[2])) > 1e-6 {
		t.Fatalf("expected components 1 and 2 to match, got %f and %f", out[1], out[2])
	}
	// Component 3 was never present in any key or value.
	if math.Abs(float64(out[3])) > 1e-6 {
		t.Fatalf("expected component 3 to be zero, got %f", out[3])
	}

	// Numeric spot-check on the dominant component.
	expHalf := float32(math.Exp(0.5))
	w0 := expHalf / (expHalf + 2)
	expected0 := w0*1 + 1 // combined[0] + query[0]
	if math.Abs(float64(out[0]-expected0)) > 1e-5 {
		t.Fatalf("component 0: expected %f, got %f", expected0, out[0])
	}
}

// TestAttentionForwardEmptyHistory verifies the degenerate path when no
// history entries are valid. The combined vector should fall back to the
// query, producing ReLU(query + query).
func TestAttentionForwardEmptyHistory(t *testing.T) {
	dim := 4
	layer := buildTestAttentionLayer(dim)

	history := make([][]float32, stateHistoryLength)
	for i := range history {
		history[i] = make([]float32, dim)
	}

	query := []float32{0.5, 0.25, 0.1, -0.3}
	out := AttentionForward(&layer, query, history, 0, 0)

	for i := range query {
		expected := query[i] * 2
		if expected < 0 {
			expected = 0
		}
		if math.Abs(float64(out[i]-expected)) > 1e-5 {
			t.Fatalf("component %d: expected %f, got %f", i, expected, out[i])
		}
	}
}

// TestAttentionForwardRingWraps verifies that when the ring buffer is full
// and histHead has advanced past slot 0, every slot contributes exactly
// once to the weighted sum. A zero query makes softmax uniform, so the
// output is the arithmetic mean of every history value.
func TestAttentionForwardRingWraps(t *testing.T) {
	dim := 4
	layer := buildTestAttentionLayer(dim)

	history := make([][]float32, stateHistoryLength)
	for i := range history {
		history[i] = make([]float32, dim)
		history[i][0] = float32(i + 1) // distinct value per slot
	}
	// Simulate wrapping: slot 0 was overwritten after the ring was full.
	history[0][0] = 99
	histHead := 1
	histLen := stateHistoryLength

	query := make([]float32, dim)
	out := AttentionForward(&layer, query, history, histLen, histHead)

	// Uniform softmax over 8 entries → mean of all slot values.
	sum := float32(0)
	for i := 0; i < stateHistoryLength; i++ {
		sum += history[i][0]
	}
	expected := sum / float32(stateHistoryLength)

	// output = ReLU(mean + zero query) = mean (values are positive).
	if math.Abs(float64(out[0]-expected)) > 1e-4 {
		t.Fatalf("component 0: expected %f, got %f", expected, out[0])
	}
	// Other components stay zero.
	for i := 1; i < dim; i++ {
		if math.Abs(float64(out[i])) > 1e-6 {
			t.Fatalf("component %d: expected 0, got %f", i, out[i])
		}
	}
}
