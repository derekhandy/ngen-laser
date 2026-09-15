package main

import (
	"math"
	"testing"
)

func TestDynamicTemperatureUsesFractionalHalfGenerations(t *testing.T) {
	rollout := &Rollout{
		Settings: RolloutSettings{
			IsTraining: true,
			Config: ExhaustiveConfig{
				DynamicTemperature: true,
				NumOfGenerations:   3,
			},
		},
		Generation: 1,
	}

	got := rollout.DynamicTemperature()
	want := float32(1.5) - (float32(1)/float32(1.5))*(float32(1.5)-float32(0.1))
	if math.Abs(float64(got-want)) > 0.0001 {
		t.Fatalf("dynamic temperature = %v, want %v", got, want)
	}
}

func TestDynamicTemperatureSingleGenerationIsFinite(t *testing.T) {
	rollout := &Rollout{
		Settings: RolloutSettings{
			IsTraining: true,
			Config: ExhaustiveConfig{
				DynamicTemperature: true,
				NumOfGenerations:   1,
			},
		},
		Generation: 0,
	}

	if got := rollout.DynamicTemperature(); math.IsNaN(float64(got)) || math.IsInf(float64(got), 0) {
		t.Fatalf("dynamic temperature is not finite: %v", got)
	}
}

func TestDynamicTemperatureUsesStaticDefaultWhenDisabled(t *testing.T) {
	rollout := &Rollout{
		Settings: RolloutSettings{
			IsTraining: true,
			Config: ExhaustiveConfig{
				DynamicTemperature: false,
				NumOfGenerations:   20,
			},
		},
		Generation: 0,
	}

	if got := rollout.DynamicTemperature(); got != 0.1 {
		t.Fatalf("temperature = %v, want 0.1", got)
	}
}

func TestConfigValidationRejectsInvalidHiddenLayerSizes(t *testing.T) {
	base := ExhaustiveConfig{
		PopulationSize:   1,
		Passes:           1,
		NumOfGenerations: 1,
		HiddenLayers:     []int{4, 2},
	}
	if !ConfigValidationPass(base) {
		t.Fatal("valid configuration was rejected")
	}
	for _, size := range []int{0, -1} {
		cfg := base
		cfg.HiddenLayers = []int{4, size}
		if ConfigValidationPass(cfg) {
			t.Fatalf("hidden layer size %d was accepted", size)
		}
	}
}
