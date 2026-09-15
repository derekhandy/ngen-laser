package main

import (
	"fmt"
	"sync"
	"testing"
)

func TestEnsureNetworkMemoryIsSafeConcurrently(t *testing.T) {
	network := InitializeNetwork(4, []int{3}, TotalOutputSize(), 0)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if EnsureNetworkMemory(network) == nil {
					t.Error("EnsureNetworkMemory returned nil")
				}
			}
		}()
	}
	wg.Wait()
}

func TestMemoryClonesRetainGRUWeights(t *testing.T) {
	cfg := RecurrentStateConfig{HiddenSize: 128, UseGRU: true}
	master := &MemoryController{
		recurrentConfig: cfg,
		state:           NewRecurrentState(cfg),
		weights:         InitializeGRUWeights(cfg.HiddenSize),
	}
	working := NewWorkingMemory(master)
	clone := CloneMemoryController(master)

	for name, controller := range map[string]*MemoryController{
		"working memory":   working,
		"controller clone": clone,
	} {
		t.Run(name, func(t *testing.T) {
			if controller == nil || controller.weights == nil {
				t.Fatal("GRU weights were not retained")
			}
			if controller.weights != master.weights {
				t.Fatal("immutable GRU weights were unnecessarily replaced")
			}

			state := &RecurrentState{
				config: cfg,
				vector: make([]float32, cfg.HiddenSize),
			}
			input := make([]float32, cfg.HiddenSize)
			for i := range input {
				input[i] = float32(i%7) / 7
				state.vector[i] = float32((i+3)%11) / 11
			}

			want := GRUUpdate(append([]float32(nil), state.vector...), input, master.weights)
			got := UpdateRecurrentState(state, input, controller.weights)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("GRU output differs at index %d: got %v, want %v", i, got[i], want[i])
				}
			}
		})
	}
}

func TestGRUUpdateUsesConfiguredDimension(t *testing.T) {
	for _, dim := range []int{64, 256} {
		t.Run(fmt.Sprintf("%d", dim), func(t *testing.T) {
			state := make([]float32, dim)
			input := make([]float32, dim)
			output := GRUUpdate(state, input, InitializeGRUWeights(dim))
			if len(output) != dim {
				t.Fatalf("GRU output length = %d, want %d", len(output), dim)
			}
		})
	}
}

func TestGRUUpdateRejectsInvalidShapes(t *testing.T) {
	if output := GRUUpdate(make([]float32, 64), make([]float32, 64), InitializeGRUWeights(128)); output != nil {
		t.Fatal("expected invalid GRU shapes to be rejected")
	}
	if ComputeGate(make([]float32, 64), make([]float32, 64), make([]float32, 64), make([]float32, 1), make([]float32, 1), make([]float32, 64)) {
		t.Fatal("expected invalid gate shapes to be rejected")
	}
}
