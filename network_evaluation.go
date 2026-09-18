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
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

func EvaluateOutput(working *CompressionEnvironment, net *Network, iterationLimit int, r *rand.Rand) (float32, string, []ChainStep, error) {
	if working == nil {
		return 0, "", nil, fmt.Errorf("compression environment is nil")
	}

	activeIterationLimit := iterationLimit

	inputBuffer := make([]float32, NetworkInputSize(net))

	masterMemory := EnsureNetworkMemory(net)

	working.memory = NewWorkingMemory(masterMemory)
	if working.memory != nil {
		working.memory.Reset()
	}

	for _, step := range working.bestChain {
		input := EncodeInstructionString(inputBuffer, working.current, ContextWindowForLength(len(working.current)), working.OperandCoverageRatio())
		if _, err := ForwardPass(net, input, working.memory); err != nil {
			return 0, "", nil, err
		}
		working.StepWithManual(step.Index, step.Operand, step.Params)
	}

	working.fitness = 0

	stepsSinceImprovement := 0

	for iteration := 0; iteration < activeIterationLimit; iteration++ {

		tokenWindow := ContextWindowForLength(len(working.current))
		coverage := working.OperandCoverageRatio()
		input := EncodeInstructionString(inputBuffer, working.current, tokenWindow, coverage)

		out, err := ForwardPass(net, input, working.memory)
		if err != nil {
			return 0, "", nil, err
		}

		beforeImprovements := working.successfulImprovements
		valid, stop := working.Step(out, activeIterationLimit, r)
		if stop {
			return working.fitness, working.current, working.currentChain, nil
		}
		if valid && working.successfulImprovements > beforeImprovements {
			successCount := working.successfulImprovements
			activeIterationLimit = CalculateLimit(iterationLimit, successCount)
			stepsSinceImprovement = 0
		} else {
			stepsSinceImprovement++
			if stepsSinceImprovement >= maxNetworkSteps {
				break
			}
		}
	}

	return working.fitness, working.current, working.currentChain, nil
}

type populationEvalJob struct {
	index   int
	network *Network
}

type populationEvalResult struct {
	index           int
	fitness         float32
	candidateString string
	bestString      string
	bestPackedSize  int
	bestChain       []ChainStep
	err             error
}

func EvaluatePopulationParallel(rollout *Rollout, size int, baseEnv *CompressionEnvironment, population []*Network, iterationLimit int) []populationEvalResult {
	size = MinInt(size, len(population))
	if size == 0 || baseEnv == nil {
		return nil
	}

	for i := 0; i < size; i++ {
		if population[i] == nil {
			continue
		}
		EnsureNetworkMemory(population[i])
	}

	workerCount := trainingWorkerLimit
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > size {
		workerCount = size
	}

	jobs := make(chan populationEvalJob)
	results := make(chan populationEvalResult, size)

	var wg sync.WaitGroup
	baseSeed := time.Now().Unix()

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			workingEnv := baseEnv.Clone()
			r := rand.New(rand.NewSource(baseSeed + int64(workerID)*7919))

			for job := range jobs {
				if job.network == nil {
					results <- populationEvalResult{
						index:   job.index,
						fitness: -1e9,
					}
					continue
				}

				masterMemory := EnsureNetworkMemory(job.network)
				if masterMemory == nil {
					results <- populationEvalResult{index: job.index, fitness: -1e9}
					continue
				}

				workingEnv.Reset()
				SeedEnvironmentWithBestChain(workingEnv)

				workingEnv.fitness = 0.0

				workingEnv.memory = NewWorkingMemory(masterMemory)

				score, str, chain, err := EvaluateOutput(workingEnv, job.network, iterationLimit, r)
				if err != nil {
					results <- populationEvalResult{index: job.index, fitness: -1e9, err: err}
					continue
				}
				bestString := workingEnv.bestString
				bestChain := workingEnv.bestChain
				bestPackedSize := workingEnv.bestPackedSize
				if bestString == "" {
					bestString = str
					bestChain = chain
					bestPackedSize = 0
				}
				if bestPackedSize <= 0 && bestString != "" && bestString != "+" {
					bestPackedSize = PackedInstructionSize(bestString)
				}

				results <- populationEvalResult{
					index:           job.index,
					fitness:         score,
					candidateString: str,
					bestString:      bestString,
					bestPackedSize:  bestPackedSize,
					bestChain:       CloneChainSteps(bestChain),
				}
			}
		}(w)
	}

	for idx, net := range population[:size] {
		jobs <- populationEvalJob{index: idx, network: net}
	}
	close(jobs)

	wg.Wait()
	close(results)

	output := make([]populationEvalResult, size)
	for res := range results {
		if res.index >= 0 && res.index < size {
			output[res.index] = res
		}
	}

	return output
}

var trainingWorkerLimit = runtime.NumCPU()

func SetTrainingWorkerLimit(n int) {
	if n < 1 {
		n = 1
	}
	trainingWorkerLimit = n
}

func ResultBestPackedSize(bestString string, fallbackPackedSize int) int {
	bestPackedSize := PackedSizeOrZero(bestString)
	if bestPackedSize > 0 {
		return bestPackedSize
	}
	return fallbackPackedSize
}

func PackedSizeOrZero(instructions string) int {
	if instructions == "" || instructions == "+" {
		return 0
	}
	return PackedInstructionSize(instructions)
}

func PacketSizeAdjustedFitness(originalPackedSize, resultPackedSize int, shapedFitness float32) float32 {
	shapedTieBreaker := BoundedShapedFitness(shapedFitness)
	if originalPackedSize <= 0 || resultPackedSize <= 0 {
		return shapedTieBreaker
	}
	packetDelta := originalPackedSize - resultPackedSize
	if packetDelta == 0 {
		return shapedTieBreaker
	}

	largerSize := MaxInt(originalPackedSize, resultPackedSize)
	savingsMagnitude := float32(math.Abs(float64(packetDelta))) / float32(largerSize)
	packetScore := packetSizeFitnessScale * (0.5 + 0.5*savingsMagnitude)
	if packetDelta < 0 {
		packetScore = -packetScore
	}

	return packetScore + shapedTieBreaker
}

func BoundedShapedFitness(fitness float32) float32 {
	fitness = NormalizedFitness(fitness)
	if math.IsInf(float64(fitness), 1) {
		return shapedFitnessTieBreakerScale
	}
	if math.IsInf(float64(fitness), -1) {
		return -shapedFitnessTieBreakerScale
	}
	return float32(math.Atan(float64(fitness)) * (shapedFitnessTieBreakerScale / (math.Pi / 2)))
}

func CloneChainSteps(chain []ChainStep) []ChainStep {
	if len(chain) == 0 {
		return nil
	}
	cloned := make([]ChainStep, len(chain))
	for i, step := range chain {
		cloned[i] = ChainStep{
			Index:   step.Index,
			Operand: step.Operand,
			Params:  CloneParams(step.Params),
		}
	}
	return cloned
}

func CalculateLimit(base int, successCount int) int {
	return base + (successCount * iterationMultiplier)
}
