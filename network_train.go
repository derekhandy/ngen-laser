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
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

func RolloutTraining(rollout *Rollout) {
	rollout.ResetMetrics()
	rollout.Settings.IsTraining = true

	ClearLogs(rollout)

	fmt.Print("Preparing training process\n")

	population, tree, err := PrepareTraining(rollout)
	if err != nil {
		LogError("preparing training rollout", err)
		return
	}

	if !PrepValidationPass(len(population), len(tree.Containers), rollout.Settings.Config) {
		LogError("validating training rollout", fmt.Errorf("population or container configuration is invalid"))
		return
	}

	fmt.Print("\n\nTraining Settings:\n\n" + GetSettingsLog(rollout))
	fmt.Print("\n\nWaiting 5 seconds to start...")

	time.Sleep(0 * time.Second)

	rollout.Generation = 0
	recordComp := float32(1.0)
	for rollout.Generation < rollout.Settings.Config.NumOfGenerations {

		if rollout.Generation > 0 {
			rollout.CurrentPassthroughLimit = rollout.Settings.Config.Passes
		}

		_population, err := TrainNetworks(rollout, population, tree)

		if err != nil {
			LogError("training canceled", err)
			return
		}

		bestNet := &Network{}
		bestFitness := float32(0)
		if len(population) > 1 {
			_population, bestNet, bestFitness, err = EvolvePopulation(_population, rollout.OverallFitnesses, rollout.Settings.Config)
			if err != nil {
				LogError("evolving population", err)
				return
			}
		}

		population = _population

		rollout.OverallFitnesses = rollout.OverallFitnesses[:0]

		if rollout.Settings.Flags.WriteGenerationAnalytics {
			if err := WriteRolloutAnalyticsGeneration(rollout, rollout.Generation, bestNet, bestFitness); err != nil {
				LogError("writing generation analytics", err)
				return
			}
		}
		rollout.Generation++
		PrintGenerationMetrics(rollout)

		currentComp := compressionRatio(rollout.Metrics.OriginalSize, rollout.Metrics.PackagedSize)
		newRecord := currentComp > 0 && currentComp < recordComp
		threshold := rollout.Settings.Flags.ThresholdCompToWrite
		thresholdReached := threshold > 0 && currentComp <= threshold
		if newRecord {
			recordComp = currentComp
			if threshold <= 0 || thresholdReached {
				if err := BuildPackage(tree); err != nil {
					LogError("writing record package", err)
					return
				}
				LogInfo("new record package written at ratio %.3f", recordComp)
			}
		}
		if thresholdReached && rollout.Settings.Flags.EndTrainingOnThreshold {
			LogInfo("compression threshold %.3f reached; ending training", threshold)
			return
		}

		nextTree, err := PrepareTree(rollout.Settings.InstructionPaths)
		if err != nil {
			LogError("preparing next training tree", err)
			return
		}
		cleanupTreeWorkspaces(tree)
		tree = nextTree

		if rollout.Settings.Flags.GenerationBasedNetworkWrite {
			interval := rollout.Settings.Flags.WriteNetworksGenerationInterval
			if interval <= 0 {
				LogError("writeNetworksGenerationInterval must be > 0", err)
				return
			}
			if rollout.Generation%interval == 0 {
				WriteNetworks(rollout, population)
				rollout.LastWriteTime = time.Now()
			}
		}
	}
}

func cleanupTreeWorkspaces(tree *Tree) {
	if tree == nil {
		return
	}
	paths := tree.TempPaths
	if len(paths) == 0 && tree.TempPath != "" {
		paths = []string{tree.TempPath}
	}
	for _, path := range paths {
		_ = os.RemoveAll(path)
	}
}

func TrainNetworks(rollout *Rollout, population []*Network, tree *Tree) ([]*Network, error) {
	monitor := NewResourceMonitor(ResourceThresholds{}, 1*time.Second)
	stopMonitor := make(chan struct{})
	runtime.GOMAXPROCS(runtime.NumCPU())
	monitor.Start(stopMonitor, func(msg string) {
	})
	defer close(stopMonitor)

	rollout.Settings.Monitor = monitor

	rollout.ResetMetrics()

	rollout.OverallFitnesses = make([]float32, rollout.Settings.Config.PopulationSize)

	for _, container := range tree.Containers {
		rollout.Metrics.ItemTotal += len(container.Lzrs)
	}

	for _, container := range tree.Containers {
		for _, lzr := range container.Lzrs {
			rollout.Metrics.ItemIndex++
			PrintGUICodes(rollout)

			if rollout.Settings.Monitor.IsCanceled() {
				return population, rollout.Settings.Monitor.Err()
			}

			inst, err := os.ReadFile(lzr)
			if err != nil {
				return population, err
			}

			env := NewCompressionEnvironment(string(inst), rollout)

			nextPopulation := ForwardPassPopulation(rollout, population, env, &container, lzr, monitor)
			rollout.Metrics.PassIndex = 0
			population = nextPopulation
		}
	}

	return population, nil
}

func ForwardPassPopulation(rollout *Rollout, population []*Network, env *CompressionEnvironment, container *Container, path string, monitor *ResourceMonitor) []*Network {
	originalString := env.originalInstructions
	originalSize := PackedInstructionSize(originalString)
	currentSize := PackedInstructionSize(originalString)

	bestString := env.originalInstructions
	bestScore := float32(-math.MaxFloat32)
	bestSize := currentSize

	var bestChain []ChainStep

	passFitnesses := make([]float32, rollout.Settings.Config.PopulationSize)

	for rollout.Metrics.PassIndex = 0; rollout.Metrics.PassIndex < rollout.CurrentPassthroughLimit; rollout.Metrics.PassIndex++ {
		startTime := time.Now()
		env.bestChain = CloneChainSteps(bestChain)

		if rollout.Settings.Monitor != nil && rollout.Settings.Monitor.IsCanceled() {
			return population
		}

		results := EvaluatePopulationParallel(rollout, rollout.Settings.Config.PopulationSize, env, population, rollout.Settings.Config.IterationLimit)
		for _, res := range results {
			resultPackedSize := res.bestPackedSize
			if resultPackedSize <= 0 {
				resultPackedSize = ResultBestPackedSize(res.bestString, originalSize)
			}
			networkFitness := PacketSizeAdjustedFitness(originalSize, resultPackedSize, res.fitness)

			passFitnesses[res.index] += networkFitness

			if res.bestString != "" && res.bestString != "+" {
				packedSize := resultPackedSize
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

		if rollout.Settings.Flags.TimeBasedNetworkWrite && (float32(time.Since(rollout.LastWriteTime).Seconds()) > float32(rollout.Settings.Flags.WriteNetworksIntervalTime)) {
			WriteNetworks(rollout, population)
			rollout.LastWriteTime = time.Now()
		}

		totalTime := time.Since(startTime)
		rollout.Metrics.PassDuration += totalTime
	}

	for i := range passFitnesses {
		rollout.OverallFitnesses[i] += passFitnesses[i] / float32(rollout.CurrentPassthroughLimit)
	}

	if err := WriteNetworkOutput(container, bestString, path); err != nil {
		LogError("writing network output", err)
		return population
	}

	rollout.Metrics.TotalComp += float32(bestSize) / float32(originalSize)

	rollout.Metrics.OriginalSize += originalSize
	rollout.Metrics.PackagedSize += bestSize

	return population
}

func WriteRolloutAnalyticsGeneration(rollout *Rollout, generation int, bestNet *Network, bestFitness float32) error {
	data := []string{
		strconv.Itoa(rollout.AnalyticsCount),
		rollout.GUID,
		fmt.Sprintf("%d", rollout.Metrics.OriginalSize),
		fmt.Sprintf("%d", rollout.Metrics.PackagedSize),
		fmt.Sprintf("%.2f", float32(rollout.Metrics.PackagedSize)/float32(rollout.Metrics.OriginalSize)),
		"-",
		fmt.Sprintf("%.2fs", float32(rollout.Metrics.PassDuration.Seconds()+0.0001)/float32((rollout.Metrics.ItemIndex)+1)),
		fmt.Sprintf("%02d:%02d", int((rollout.Metrics.PassDuration.Seconds() / 60)), int(rollout.Metrics.PassDuration.Seconds())%60),
		fmt.Sprintf("%2d", int(time.Since(rollout.StartTime).Seconds())),
		"-",
		fmt.Sprintf("%02d", int(rollout.Settings.Config.PopulationSize)),
		fmt.Sprintf("%02d", int(rollout.Settings.Config.IterationLimit)),
		fmt.Sprintf("%02d", int(rollout.Settings.Config.Passes)),
		fmt.Sprintf("%.2f", rollout.Settings.Config.ElitePercent),
		fmt.Sprintf("%.2f", rollout.Settings.Config.MutatedElitePercent),
		fmt.Sprintf("%.2f", rollout.Settings.Config.NewInitPercent),
		fmt.Sprintf("%d", int(rollout.Settings.Config.InputSize)),
		fmt.Sprintf("%v", rollout.Settings.Config.HiddenLayers),
		fmt.Sprintf("%d", latentSize),
		fmt.Sprintf("%d", inputTokenStride),
		fmt.Sprintf("%.2f", rollout.DynamicTemperature()),
		"-",
		bestNet.networkName,
		fmt.Sprintf("%.2f", bestFitness),
		fmt.Sprintf("%v", rollout.Settings.InstructionPaths),
	}

	content := "\n"
	for i := range len(data) {
		content += data[i] + ","
	}

	content = content[:len(content)-1]

	analyticsDir := rollout.Paths.AnalyticsDir(rollout.SafeAnalyticsGUID())

	if err := os.MkdirAll(analyticsDir, 0755); err != nil {
		return fmt.Errorf("create analytics directory: %w", err)
	}
	if rollout.AnalyticsCount == 0 {
		content = analyticsHeader + content
	}

	file, err := os.OpenFile(filepath.Join(analyticsDir, "analytics.csv"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open analytics file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("write analytics file: %w", err)
	}

	rollout.AnalyticsCount++
	return nil
}

func DebugPrint(rollout *Rollout, str string, override bool) {
	if rollout.Settings.Flags.DebugMode || override {
		ClearLogs(rollout)
		fmt.Print(str)
		DebugPause()
	}
}

func DebugPause() {
	fmt.Print("\n\nPress 'Enter' to continue...")

	_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
}
