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
)

type RecurrentStateConfig struct {
	HiddenSize int
	UseGRU     bool
	UseLSTM    bool
}

type GRUWeights struct {
	Wz, Uz, Bz []float32
	Wr, Ur, Br []float32
	Wh, Uh, Bh []float32
}

type RecurrentState struct {
	config RecurrentStateConfig
	vector []float32
}

func NewRecurrentState(cfg RecurrentStateConfig) *RecurrentState {
	if cfg.HiddenSize < 0 {
		cfg.HiddenSize = 0
	}
	return &RecurrentState{
		config: cfg,
		vector: make([]float32, cfg.HiddenSize),
	}
}

func DefaultRecurrentStateConfig() RecurrentStateConfig {
	return RecurrentStateConfig{
		HiddenSize: latentSize,
		UseGRU:     true,
	}
}

func (state *RecurrentState) Reset() {
	clear(state.vector)
}

func GRUUpdate(state []float32, input []float32, w *GRUWeights) []float32 {
	if len(state) == 0 {
		return input
	}
	dim := len(state)
	if len(input) != dim || !ValidGRUWeights(w, dim) {
		return nil
	}

	z := make([]float32, dim)
	if !ComputeGate(z, input, state, w.Wz, w.Uz, w.Bz) {
		return nil
	}
	for i := range z {
		z[i] = Sigmoid(z[i])
	}

	r := make([]float32, dim)
	if !ComputeGate(r, input, state, w.Wr, w.Ur, w.Br) {
		return nil
	}
	for i := range r {
		r[i] = Sigmoid(r[i])
	}

	resetState := make([]float32, dim)
	for i := range state {
		resetState[i] = r[i] * state[i]
	}
	hHat := make([]float32, dim)
	if !ComputeGate(hHat, input, resetState, w.Wh, w.Uh, w.Bh) {
		return nil
	}
	for i := range hHat {
		hHat[i] = Tanh(hHat[i])
	}

	output := make([]float32, dim)
	for i := range output {
		output[i] = (1-z[i])*state[i] + z[i]*hHat[i]
	}
	return output
}

func InitializeGRUWeights(dim int) *GRUWeights {
	if dim <= 0 {
		return nil
	}
	w := &GRUWeights{
		Wz: make([]float32, dim*dim), Uz: make([]float32, dim*dim), Bz: make([]float32, dim),
		Wr: make([]float32, dim*dim), Ur: make([]float32, dim*dim), Br: make([]float32, dim),
		Wh: make([]float32, dim*dim), Uh: make([]float32, dim*dim), Bh: make([]float32, dim),
	}

	stdDev := math.Sqrt(1.0 / float64(dim))

	initWeight := func(slice []float32) {
		for i := range slice {
			slice[i] = float32(rand.NormFloat64() * stdDev)
		}
	}

	initWeight(w.Wz)
	initWeight(w.Uz)
	initWeight(w.Wr)
	initWeight(w.Ur)
	initWeight(w.Wh)
	initWeight(w.Uh)

	for i := range w.Bz {
		w.Bz[i] = -1.0
	}

	return w
}

func ComputeGate(out, x, h, W, U, b []float32) bool {
	dim := len(out)
	if dim == 0 || len(x) != dim || len(h) != dim || len(b) != dim || len(W) != dim*dim || len(U) != dim*dim {
		return false
	}
	for i := 0; i < dim; i++ {
		var sum float32
		rowOffset := i * dim
		for j := 0; j < dim; j++ {
			sum += W[rowOffset+j]*x[j] + U[rowOffset+j]*h[j]
		}
		out[i] = sum + b[i]
	}
	return true
}

func ValidGRUWeights(w *GRUWeights, dim int) bool {
	if w == nil || dim <= 0 {
		return false
	}
	return len(w.Wz) == dim*dim && len(w.Uz) == dim*dim && len(w.Bz) == dim &&
		len(w.Wr) == dim*dim && len(w.Ur) == dim*dim && len(w.Br) == dim &&
		len(w.Wh) == dim*dim && len(w.Uh) == dim*dim && len(w.Bh) == dim
}

func UpdateRecurrentState(state *RecurrentState, input []float32, weights *GRUWeights) []float32 {
	if state == nil {
		return input
	}

	if state.config.UseGRU && weights != nil {
		if output := GRUUpdate(state.vector, input, weights); output != nil {
			state.vector = output
			return state.vector
		}
	}

	if len(state.vector) != len(input) {
		state.vector = make([]float32, len(input))
	}
	copy(state.vector, input)
	return state.vector
}
