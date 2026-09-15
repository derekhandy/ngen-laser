package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadNetworkRejectsMalformedLayerShapes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed.json")
	data := []byte(`{
  "networkName": "malformed",
  "inputSize": 2,
  "backbone": [{
    "weights": [[1, 2], [3]],
    "biases": [0, 0],
    "activation": "relu"
  }]
}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadNetwork(path); err == nil {
		t.Fatal("expected malformed layer to be rejected")
	} else if !strings.Contains(err.Error(), "backbone[0] weight row 1") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestNetworkRoundTripPreservesGRUWeights(t *testing.T) {
	net := InitializeNetwork(128, []int{128}, TotalOutputSize(), 0)
	net.memory.weights.Wz[0] = 1.25
	net.memory.weights.Ur[17] = -2.5
	net.memory.weights.Bh[3] = 0.75

	path := filepath.Join(t.TempDir(), "network.json")
	if err := SaveNetwork(path, net); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadNetwork(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(net.memory.weights, loaded.memory.weights) {
		t.Fatal("GRU weights changed during network round trip")
	}
}

func TestValidateSerializableNetworkRejectsMissingGRUWeights(t *testing.T) {
	payload := SerializableNetwork{
		InputSize: 1,
		Backbone:  []SerialLayer{{Weights: [][]float32{{0}}, Biases: []float32{0}}},
		Heads: SerialHeads{
			Index:            SerialLayer{Weights: [][]float32{{0}}, Biases: []float32{0}},
			Operand:          SerialLayer{Weights: makeRows(len(operandClasses)), Biases: make([]float32, len(operandClasses))},
			TranslationDir:   SerialLayer{Weights: makeRows(len(directionClasses)), Biases: make([]float32, len(directionClasses))},
			TranslationMag:   SerialLayer{Weights: [][]float32{{0}}, Biases: []float32{0}},
			RotationAxis:     SerialLayer{Weights: makeRows(len(axisClasses)), Biases: make([]float32, len(axisClasses))},
			ScalingMagnitude: SerialLayer{Weights: [][]float32{{0}}, Biases: []float32{0}},
			InsertionLength:  SerialLayer{Weights: [][]float32{{0}}, Biases: []float32{0}},
			Stop:             SerialLayer{Weights: [][]float32{{0}}, Biases: []float32{0}},
		},
		Memory: &SerialMemory{
			RecurrentConfig: RecurrentStateConfig{HiddenSize: 1, UseGRU: true},
			AttentionConfig: AttentionConfig{HiddenSize: 1, NumHeads: 1},
			LayerCount:      1,
			Encoder: []SerialAttentionLayer{{
				Config: AttentionConfig{HiddenSize: 1, NumHeads: 1},
				QProj:  serialIdentityLayer(), KProj: serialIdentityLayer(), VProj: serialIdentityLayer(),
				OProj: serialIdentityLayer(), FFN1: serialIdentityLayer(), FFN2: serialIdentityLayer(),
			}},
		},
	}
	if err := ValidateSerializableNetwork(payload); err == nil || !strings.Contains(err.Error(), "GRU weights are missing") {
		t.Fatalf("expected missing GRU weights error, got %v", err)
	}
}

func makeRows(count int) [][]float32 {
	rows := make([][]float32, count)
	for i := range rows {
		rows[i] = []float32{0}
	}
	return rows
}

func serialIdentityLayer() SerialLayer {
	return SerialLayer{Weights: [][]float32{{1}}, Biases: []float32{0}}
}
