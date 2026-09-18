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
	"os"
	"runtime"
)

func main() {
	os.Exit(run(os.Args))
}

func run(args []string) int {
	runtime.GOMAXPROCS(runtime.NumCPU())
	trainingSizeMode = SizeModeBitOps
	packagingCodec = CodecRaw

	if len(args) < 2 {
		PrintUsage()
		return 1
	}

	dataDirOverride, args, err := parseDataDirFlag(args)
	if err != nil {
		LogError("flags", err)
		return 1
	}

	codecOverride, args, err := parseCodecFlag(args)
	if err != nil {
		LogError("codec flag", err)
		return 1
	}
	if codecOverride != nil {
		packagingCodec = *codecOverride
	}

	cpuOverride, args, err := parseCPUFlag(args)
	if err != nil {
		LogError("cpus flag", err)
		return 1
	}
	if cpuOverride > 0 {
		runtime.GOMAXPROCS(cpuOverride)
		SetTrainingWorkerLimit(cpuOverride)
	} else {
		runtime.GOMAXPROCS(runtime.NumCPU())
		SetTrainingWorkerLimit(runtime.NumCPU())
	}

	isGui := false
	if args[len(args)-1] == "-gui" {
		isGui = true
		args = args[:len(args)-1]
	}
	if len(args) < 2 {
		PrintUsage()
		return 1
	}

	SetColorValues(isGui)

	command := args[1]

	switch command {
	case "train":
		return runTrain(args, dataDirOverride, isGui)
	case "pack":
		return runPack(args, dataDirOverride, isGui)
	case "unpack":
		return runUnpack(args)
	default:
		PrintUsage()
		return 1
	}
}

func runTrain(args []string, dataDirOverride string, isGui bool) int {
	if len(args) < 3 {
		PrintUsage()
		return 1
	}

	thresholdOverride, trainArgs, err := parseThresholdFlag(args)
	if err != nil {
		LogError("threshold flag", err)
		return 1
	}

	useDeflate, trainArgs, err := parseDeflateFitnessFlag(trainArgs)
	if err != nil {
		LogError("deflate fitness flag", err)
		return 1
	}
	if useDeflate {
		trainingSizeMode = SizeModeDeflate
	} else {
		trainingSizeMode = SizeModeBitOps
	}

	if len(trainArgs) < 3 {
		PrintUsage()
		return 1
	}

	rollout, err := newCommandRollout(dataDirOverride, isGui)
	if err != nil {
		LogError("data directory", err)
		return 1
	}
	if thresholdOverride != nil {
		rollout.ThresholdCompOverride = thresholdOverride
	}
	if err := rollout.SetInstructionPath(trainArgs[2]); err != nil {
		LogError("instruction path", err)
		return 1
	}
	RolloutTraining(rollout)
	return 0
}

func runPack(args []string, dataDirOverride string, isGui bool) int {
	if len(args) > 2 && args[2] == "-c" {
		if len(args) < 5 {
			PrintUsage()
			return 1
		}
		rollout, err := newCommandRollout(dataDirOverride, isGui)
		if err != nil {
			LogError("data directory", err)
			return 1
		}
		if err := rollout.SetInstructionPath(args[3]); err != nil {
			LogError("instruction path", err)
			return 1
		}
		if err := ComputeCompression(rollout, args[4]); err != nil {
			LogError("compute pack", err)
			return 1
		}
		return 0
	}

	if len(args) < 3 {
		PrintUsage()
		return 1
	}

	rollout, err := newCommandRollout(dataDirOverride, isGui)
	if err != nil {
		LogError("data directory", err)
		return 1
	}
	if err := rollout.SetInstructionPath(args[2]); err != nil {
		LogError("instruction path", err)
		return 1
	}
	RolloutPackaging(rollout)
	return 0
}

func runUnpack(args []string) int {
	if len(args) < 3 {
		PrintUsage()
		return 1
	}
	if err := RestorePackage(args[2]); err != nil {
		LogError("unpack", err)
		return 1
	}
	return 0
}

func newCommandRollout(dataDirOverride string, isGUI bool) (*Rollout, error) {
	paths, err := ResolveDataRoot(dataDirOverride)
	if err != nil {
		return nil, err
	}
	return NewRollout(paths, isGUI), nil
}

func PrintUsage() {
	fmt.Printf("\n\nlaser %s\n\t\t", version)
	fmt.Print("\n\t|\ttrain <path> [-t <comp-ratio>] [--deflate-fitness]\t\n\t|\tTrain neural networks on data at <path>.\n")
	fmt.Print("\n\n\t|\tpack <path>\t\n\t|\tUse saved networks to compress data at <path>.\n")
	fmt.Print("\n\n\t|\tpack -c <path> <var-max>,<iteration-max>\t\n\t|\tCompute compression with saved networks.\n")
	fmt.Print("\n\n\t|\tunpack <file>\t\n\t|\tDecompresses .lzr files back to original.\n")
	fmt.Print("\n\n\t|\t--data-dir <path>\t\n\t|\tOverride the data directory (config, weights, analytics).\n")
	fmt.Print("\n\n\t|\t--cpus <n>\t\n\t|\tLimit concurrent workers (default: number of CPU cores).\n")
	fmt.Print("\n\n\t|\t--deflate-fitness\t\n\t|\tUse DEFLATE size as the compression metric. Default: bit-ops size.\n")
	fmt.Print("\n\n\t|\t--codec <name>\t\n\t|\tDownstream compressor for pack. Default: none.\n")
	fmt.Print("\n\n")
}
