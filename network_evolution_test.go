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

import "testing"

func TestValidatePopulationPercentagesAllowsFloatRounding(t *testing.T) {
	cfg := ExhaustiveConfig{
		ElitePercent:        0.1,
		MutatedElitePercent: 0.2,
		CrossoverPercent:    0.3,
		NewInitPercent:      0.40000004,
	}
	if err := ValidatePopulationPercentages(cfg); err != nil {
		t.Fatalf("near-one percentages were rejected: %v", err)
	}
}

func TestValidatePopulationPercentagesRejectsOutOfRangeValues(t *testing.T) {
	base := ExhaustiveConfig{
		ElitePercent:        0.25,
		MutatedElitePercent: 0.25,
		CrossoverPercent:    0.25,
		NewInitPercent:      0.25,
	}
	for _, test := range []struct {
		name   string
		mutate func(*ExhaustiveConfig)
	}{
		{name: "negative", mutate: func(cfg *ExhaustiveConfig) { cfg.ElitePercent = -0.01 }},
		{name: "above one", mutate: func(cfg *ExhaustiveConfig) { cfg.ElitePercent = 1.01 }},
		{name: "sum too low", mutate: func(cfg *ExhaustiveConfig) { cfg.NewInitPercent = 0.1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			test.mutate(&cfg)
			if err := ValidatePopulationPercentages(cfg); err == nil {
				t.Fatal("invalid percentages were accepted")
			}
		})
	}
}

func TestEvolvePopulationValidatesBeforeCounting(t *testing.T) {
	population := []*Network{InitializeNetwork(4, []int{3}, TotalOutputSize(), 0)}
	cfg := ExhaustiveConfig{ElitePercent: 2, MutatedElitePercent: -1, CrossoverPercent: 0, NewInitPercent: 0}
	if _, _, _, err := EvolvePopulation(population, []float32{0}, cfg); err == nil {
		t.Fatal("out-of-range percentages were accepted")
	}
}

func TestEvolvePopulationFillsEverySlot(t *testing.T) {
	population := make([]*Network, 5)
	for i := range population {
		population[i] = InitializeNetwork(4, []int{3}, TotalOutputSize(), i)
	}

	config := ExhaustiveConfig{
		ElitePercent:        0.25,
		MutatedElitePercent: 0.25,
		CrossoverPercent:    0.25,
		NewInitPercent:      0.25,
	}
	result, _, _, err := EvolvePopulation(population, make([]float32, len(population)), config)
	if err != nil {
		t.Fatalf("EvolvePopulation returned an unexpected error: %v", err)
	}
	for i, network := range result {
		if network == nil {
			t.Fatalf("result slot %d is nil", i)
		}
	}
}

func TestEvolvePopulationHandlesNilParents(t *testing.T) {
	config := ExhaustiveConfig{
		ElitePercent:     0.25,
		CrossoverPercent: 0.25,
		NewInitPercent:   0.5,
	}
	result, _, _, err := EvolvePopulation(make([]*Network, 4), make([]float32, 4), config)
	if err != nil {
		t.Fatalf("EvolvePopulation returned an unexpected error: %v", err)
	}
	for i, network := range result {
		if network == nil {
			t.Fatalf("result slot %d is nil", i)
		}
	}
}

func TestCrossoverHandlesNilParent(t *testing.T) {
	parent := InitializeNetwork(4, []int{3}, TotalOutputSize(), 0)
	if Crossover(nil, parent) == nil {
		t.Fatal("expected crossover to clone the non-nil parent")
	}
	if Crossover(parent, nil) == nil {
		t.Fatal("expected crossover to clone the non-nil parent")
	}
}
