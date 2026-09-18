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
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Purple = "\033[35m"
	Cyan   = "\033[36m"
	White  = "\033[37m"
)

func PrintPassLogs(rollout *Rollout) {
	elapsed := time.Since(rollout.StartTime)
	completedCycles := rollout.Settings.Config.NumOfGenerations - (rollout.Settings.Config.NumOfGenerations - rollout.Generation)

	var timeLeft time.Duration
	if completedCycles > 0 {
		timeLeft = (elapsed / time.Duration(completedCycles)) * time.Duration(rollout.Settings.Config.NumOfGenerations-rollout.Generation)
	}

	if rollout.Settings.IsGUI {
		PrintGUICodes(rollout)
		return
	}

	ratio := compressionRatio(rollout.Metrics.OriginalSize, rollout.Metrics.PackagedSize)
	fmt.Printf("\rGen %d/%d | Part %d/%d | %dB -> %dB | ratio %.2f | ETA %02d:%02d:%02d",
		rollout.Generation, rollout.Settings.Config.NumOfGenerations,
		rollout.Metrics.ItemIndex, rollout.Metrics.ItemTotal,
		rollout.Metrics.OriginalSize, rollout.Metrics.PackagedSize, ratio,
		int(timeLeft.Minutes()/60), int(timeLeft.Minutes())%60, int(timeLeft.Seconds())%60)
}

func PrintGenerationMetrics(rollout *Rollout) {
	if rollout.Settings.IsGUI {
		PrintGUICodes(rollout)
		return
	}
	fmt.Printf("\nGeneration %d/%d complete | parts %d/%d | %dB -> %dB | ratio %.2f | compression %.1f%% | elapsed %s\n",
		rollout.Generation, rollout.Settings.Config.NumOfGenerations,
		rollout.Metrics.ItemIndex, rollout.Metrics.ItemTotal,
		rollout.Metrics.OriginalSize, rollout.Metrics.PackagedSize,
		compressionRatio(rollout.Metrics.OriginalSize, rollout.Metrics.PackagedSize),
		compressionPercent(rollout.Metrics.OriginalSize, rollout.Metrics.PackagedSize),
		formatDuration(rollout.Metrics.PassDuration))
}

func compressionRatio(originalSize, packagedSize int) float32 {
	if originalSize <= 0 {
		return 0
	}
	return float32(packagedSize) / float32(originalSize)
}

func compressionPercent(originalSize, packagedSize int) float32 {
	return (1 - compressionRatio(originalSize, packagedSize)) * 100
}

func formatDuration(duration time.Duration) string {
	return fmt.Sprintf("%02d:%02d:%02d", int(duration.Hours()), int(duration.Minutes())%60, int(duration.Seconds())%60)
}

func GetSettingsLog(rollout *Rollout) string {
	setString := "" +

		fmt.Sprintf("\tGenerations \t%d\n", rollout.Settings.Config.NumOfGenerations) +
		fmt.Sprintf("\tPopulation  \t%d\n", rollout.Settings.Config.PopulationSize) +
		fmt.Sprintf("\tPasses      \t%d\n", rollout.Settings.Config.Passes) +
		fmt.Sprintf("\tDynamic Temp.\t%t\n", rollout.Settings.Config.DynamicTemperature) +

		"\n" +

		fmt.Sprintf("\tInstructions\t%v\n", rollout.Settings.InstructionPaths) +

		"\n\n" +

		fmt.Sprintf("[*] Write analytics every generation: %v\n",
			strings.ToUpper(strconv.FormatBool(rollout.Settings.Flags.WriteGenerationAnalytics))) +

		fmt.Sprintf("[*] Write networks every *%d* generations: %v\n",
			rollout.Settings.Flags.WriteNetworksGenerationInterval,
			strings.ToUpper(strconv.FormatBool(rollout.Settings.Flags.GenerationBasedNetworkWrite))) +

		fmt.Sprintf("[*] Write networks every *%d* seconds: %v\n",
			rollout.Settings.Flags.WriteNetworksIntervalTime,
			strings.ToUpper(strconv.FormatBool(rollout.Settings.Flags.TimeBasedNetworkWrite)))

	return setString
}

func PrintGUICodes(rollout *Rollout) {
	if !rollout.Settings.IsGUI {
		return
	}
	fmt.Printf("[CCYL]_%d.%d", rollout.Generation, rollout.Settings.Config.NumOfGenerations)
	fmt.Printf("[GNRA]_%d.%d", rollout.Metrics.PassIndex, rollout.Settings.Config.Passes)
	fmt.Printf("[LRND]_%d.%d", rollout.Metrics.ItemIndex, rollout.Metrics.ItemTotal)
}

func ClearLogs(rollout *Rollout) {
	if rollout != nil && rollout.Settings.IsGUI {
		fmt.Print("[CLR_STRM]")
	} else {
		ClearConsole()
	}
}

func ClearConsole() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error occurred while clearing console: %v\n", err)
	}
}

var r, g, y, w = "", "", "", ""

func SetColorValues(isGUI bool) {
	if !isGUI {
		r = Red
		g = Green
		y = Yellow
		w = White
	}
}

func PrintConfirm(t string) {
	switch t {
	case "done":
		fmt.Print(y + "[DONE]" + w + "\n")
	case "pass":
		fmt.Print(g + "[PASS]" + w + "\n")
	case "fail":
		fmt.Print(r + "[FAIL]" + w + "\n")
	}
}

func LogInfo(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}

func LogWarning(format string, args ...interface{}) {
	fmt.Printf(y+"[WARN]"+w+" "+format+"\n", args...)
}

func LogError(context string, err error) {
	if err == nil {
		fmt.Printf(r+"[ERROR]"+w+" %s\n", context)
		return
	}
	fmt.Printf(r+"[ERROR]"+w+" %s: %v\n", context, err)
}

func LogLoading(label string) {
	fmt.Printf("[INFO] %s... ", label)
}
