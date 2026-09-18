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
	"os"
	"runtime"
	"time"
)

func RolloutPackaging(rollout *Rollout) {
	rollout.ResetMetrics()
	rollout.Settings.IsTraining = false

	ClearLogs(rollout)

	fmt.Print("\nPreparing packaging process ->\n")

	population, tree, err := PreparePackaging(rollout)

	if err != nil {
		LogError("preparing packaging rollout", err)
		return
	}

	if !PrepValidationPass(len(population), len(tree.Containers), rollout.Settings.Config) {
		LogError("validating packaging rollout", fmt.Errorf("population or container configuration is invalid"))
		return
	}

	fmt.Print("\n\nPackaging Settings:\n\n" + GetSettingsLog(rollout))
	fmt.Print("\n\nWaiting 5 seconds to start...")

	time.Sleep(5 * time.Second)

	RolloutGenerations(rollout, population, tree)
}

func RolloutGenerations(rollout *Rollout, population []*Network, tree *Tree) {
	recordComp := float32(1.0)

	rollout.Generation = 0
	for rollout.Generation < rollout.Settings.Config.NumOfGenerations {
		if rollout.Generation > 0 {
			rollout.CurrentPassthroughLimit = rollout.Settings.Config.Passes
		}

		err := RolloutForwardPass(rollout, population, tree)
		if err != nil {
			LogError("rollout forward pass", err)
			return
		}
		bestNet := &Network{networkName: "none"}
		bestFitness := float32(0)

		if rollout.Settings.Flags.WriteGenerationAnalytics {
			if err := WriteRolloutAnalyticsGeneration(rollout, rollout.Generation, bestNet, bestFitness); err != nil {
				LogError("writing generation analytics", err)
				return
			}
		}

		rollout.Generation++
		PrintGenerationMetrics(rollout)

		if (float32(rollout.Metrics.PackagedSize) / float32(rollout.Metrics.OriginalSize)) < recordComp {
			recordComp = (float32(rollout.Metrics.PackagedSize) / float32(rollout.Metrics.OriginalSize))
			if err := BuildPackage(tree); err != nil {
				LogError("building package", err)
				return
			}
			fmt.Printf("\n\nBuilt package with comp%%: %.2f\n", recordComp)
			time.Sleep(2 * time.Second)
		}

		_tree, err := PrepareTree(rollout.Settings.InstructionPaths)
		if err != nil {
			LogError("preparing next packaging tree", err)
			return
		}

		tree = _tree
	}
}

func RolloutForwardPass(rollout *Rollout, population []*Network, tree *Tree) error {
	monitor := NewResourceMonitor(ResourceThresholds{}, 1*time.Second)
	stopMonitor := make(chan struct{})
	runtime.GOMAXPROCS(runtime.NumCPU())
	monitor.Start(stopMonitor, func(msg string) {
	})
	defer close(stopMonitor)

	rollout.Settings.Monitor = monitor

	rollout.ResetMetrics()

	err := PackageLzrs(rollout, population, tree)
	if err != nil {
		return fmt.Errorf("[!] error in PackageLzrs(): %v", err)
	}

	return nil
}

func PackageLzrs(rollout *Rollout, population []*Network, tree *Tree) error {
	for _, container := range tree.Containers {
		rollout.Metrics.ItemTotal += len(container.Lzrs)
	}

	for containerIndex := range tree.Containers {
		for _, lzr := range tree.Containers[containerIndex].Lzrs {
			rollout.Metrics.ItemIndex++
			PrintGUICodes(rollout)

			if rollout.Settings.Monitor.IsCanceled() {
				return rollout.Settings.Monitor.Err()
			}

			inst, err := os.ReadFile(lzr)
			if err != nil {
				return err
			}

			env := NewCompressionEnvironment(string(inst), rollout)

			err = ForwardPassInstructions(rollout, population, env, &tree.Containers[containerIndex], lzr)
			if err != nil {
				return fmt.Errorf("[!] error in ForwardPassInstructions(): %v", err)
			}
			rollout.Metrics.PassIndex = 0
		}
	}

	return nil
}

func ForwardPassInstructions(rollout *Rollout, population []*Network, env *CompressionEnvironment, container *Container, path string) error {
	originalString := env.originalInstructions
	originalSize := PackedInstructionSize(originalString)
	currentSize := PackedInstructionSize(originalString)

	bestScore := float32(-math.MaxFloat32)
	bestString := originalString
	bestSize := currentSize

	var bestChain []ChainStep

	for rollout.Metrics.PassIndex = 0; rollout.Metrics.PassIndex < rollout.CurrentPassthroughLimit; rollout.Metrics.PassIndex++ {
		startTime := time.Now()
		env.bestChain = CloneChainSteps(bestChain)

		if rollout.Settings.Monitor != nil && rollout.Settings.Monitor.IsCanceled() {
			return fmt.Errorf("[!] Monitor was canceled")
		}

		results := EvaluatePopulationParallel(rollout, rollout.Settings.Config.PopulationSize, env, population, rollout.Settings.Config.IterationLimit)
		for _, res := range results {
			resultPackedSize := ResultBestPackedSize(res.bestString, originalSize)
			networkFitness := PacketSizeAdjustedFitness(
				originalSize,
				resultPackedSize,
				res.fitness,
			)

			if res.bestString != "" && res.bestString != "+" {
				packedSize := PackedInstructionSize(res.bestString)
				if packedSize < bestSize {
					bestString = res.bestString
					bestSize = packedSize
					bestScore = networkFitness
					bestChain = CloneChainSteps(res.bestChain)

				} else if packedSize == bestSize && networkFitness > bestScore {
					bestString = res.bestString
					bestSize = packedSize
					bestScore = networkFitness
					bestChain = CloneChainSteps(res.bestChain)
				}
			}
		}

		PrintPassLogs(rollout)

		totalTime := time.Since(startTime)
		rollout.Metrics.PassDuration += totalTime
	}

	if err := WriteNetworkOutput(container, bestString, path); err != nil {
		return err
	}

	rollout.Metrics.TotalComp += float32(bestSize) / float32(originalSize)

	rollout.Metrics.OriginalSize += originalSize
	rollout.Metrics.PackagedSize += bestSize

	return nil
}
