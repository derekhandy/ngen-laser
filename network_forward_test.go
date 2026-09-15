package main

import (
	"math"
	"testing"
)

func TestEncodeInstructionStringAddsPositionalVariation(t *testing.T) {
	const tokenCount = 4
	input := make([]float32, additionalFeatureSize+inputTokenStride*tokenCount)
	EncodeInstructionString(input, ";;;;", tokenCount, 0)

	positions := make([]float32, tokenCount)
	for i := range positions {
		positions[i] = input[i*inputTokenStride+4]
	}
	if math.Abs(float64(positions[1])) < 0.1 || math.Abs(float64(positions[3])) < 0.1 {
		t.Fatalf("positional features are effectively zero: %v", positions)
	}
	if positions[1] == positions[3] {
		t.Fatalf("positional features did not vary by position: %v", positions)
	}
}

func TestForwardPassRejectsInvalidState(t *testing.T) {
	if _, err := ForwardPass(nil, nil, nil); err == nil {
		t.Fatal("expected nil network to be rejected")
	}

	net := InitializeNetwork(4, []int{3}, TotalOutputSize(), 0)
	memory := NewMemoryController(DefaultMemoryConfig(3))
	net.backbone[0].biases = nil
	if _, err := ForwardPass(net, make([]float32, 4), memory); err == nil {
		t.Fatal("expected inconsistent layer shape to be rejected")
	}
}

func TestForwardPassAllocatesMissingBuffers(t *testing.T) {
	net := InitializeNetwork(4, []int{3}, TotalOutputSize(), 0)
	memory := NewMemoryController(DefaultMemoryConfig(3))

	out, err := ForwardPass(net, make([]float32, 4), memory)
	if err != nil {
		t.Fatalf("ForwardPass returned an unexpected error: %v", err)
	}
	if len(out.Operand.Logits) != len(operandClasses) {
		t.Fatalf("expected %d operand logits, got %d", len(operandClasses), len(out.Operand.Logits))
	}
}
