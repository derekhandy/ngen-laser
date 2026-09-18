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

import "math"

type Vector3 struct {
	X, Y, Z float32
}

type Vector3Int struct {
	X, Y, Z int
}

func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ClampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func Clamp(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func FastTanh(x float32) float32 {
	if x >= 3.0 {
		return 1.0
	}
	if x <= -3.0 {
		return -1.0
	}
	x2 := x * x
	return x * (27.0 + x2) / (27.0 + 9.0*x2)
}

func Sigmoid(x float32) float32 {
	if x > 8 {
		return 1.0
	}
	if x < -8 {
		return 0.0
	}
	return 1.0 / (1.0 + float32(math.Exp(float64(-x))))
}

func Relu(x float32) float32 {
	if x > 0 {
		return x
	}
	return 0
}

func Tanh(x float32) float32 {
	return float32(math.Tanh(float64(x)))
}
