//
//													  LASER
//
//									 MIT License, Copyright (c) 2026 Derek Handy
//						 Project can be found at: https://github.com/derekhandy/ngen-laser
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

	if len(args) < 2 {
		PrintUsage()
		return 1
	}

	dataDirOverride, args, err := parseDataDirFlag(args)
	if err != nil {
		LogError("flags", err)
		return 1
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
		if len(args) < 3 {
			PrintUsage()
			return 1
		}

		thresholdOverride, trainArgs, err := parseThresholdFlag(args)
		if err != nil {
			LogError("threshold flag", err)
			return 1
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

	case "pack":
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
		} else if len(args) < 3 {
			PrintUsage()
			return 1
		} else {
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
		}

	case "unpack":
		if len(args) < 3 {
			PrintUsage()
			return 1
		}
		if err := RestorePackage(args[2]); err != nil {
			LogError("unpack", err)
			return 1
		}

	default:
		PrintUsage()
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
	fmt.Print("\n\t|\ttrain <path> [-t <comp-ratio>]\t\n\t|\tTrain neural networks on data at <path>.")
	fmt.Print("\n\n\t|\tpack <path>\t\n\t|\tUse saved networks to compress data at <path>.")
	fmt.Print("\n\n\t|\tpack -c <path> <var-max>,<iteration-max>\t\n\t|\tCompute compression with saved networks.")
	fmt.Print("\n\n\t|\tunpack <file>\t\n\t|\tDecompresses .lzr files back to original.")
	fmt.Print("\n\n\t|\t--data-dir <path>\t\n\t|\tOverride the data directory (config, weights, analytics).")
	fmt.Print("\n\n")
}
