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
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AppPaths struct {
	Root string
}

func (p AppPaths) Config(name string) string {
	return filepath.Join(p.Root, "config", name)
}

func (p AppPaths) Weights(name string) string {
	return filepath.Join(p.Root, "weights", name)
}

func (p AppPaths) WeightsDir() string {
	return filepath.Join(p.Root, "weights")
}

func (p AppPaths) AnalyticsDir(safeGUID string) string {
	return filepath.Join(p.Root, "analytics", safeGUID)
}

// TrainingSizeMode selects which size function the fitness comparison uses.
// SizeModeBitOps is the fast path and matches the pre-DEFLATE behavior.
// SizeModeDeflate runs a real DEFLATE compression on each candidate and is
// roughly 5-10x slower, but aligns the training signal with the shipped
// per-container DEFLATE format.
type TrainingSizeMode int

var packagingCodec = CodecDeflate

const (
	SizeModeBitOps TrainingSizeMode = iota
	SizeModeDeflate
)

var trainingSizeMode = SizeModeBitOps

type Rollout struct {
	Paths    AppPaths
	Settings RolloutSettings

	Metrics                 RolloutMetrics
	Generation              int
	OverallFitnesses        []float32
	CurrentPassthroughLimit int
	LastWriteTime           time.Time
	StartTime               time.Time
	GUID                    string
	AnalyticsCount          int
	AnalyticsGUID           string

	ThresholdCompOverride *float32
}

func NewRollout(paths AppPaths, isGUI bool) *Rollout {
	return &Rollout{
		Paths: paths,
		Settings: RolloutSettings{
			IsGUI: isGUI,
		},
		CurrentPassthroughLimit: 1,
	}
}

func (r *Rollout) SetInstructionPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("instruction path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("instruction path %q does not exist", path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("instruction path %q is not readable", path)
		}
		return fmt.Errorf("instruction path %q: %w", path, err)
	}
	r.Settings.InstructionPaths = []string{path}
	return nil
}

func (r *Rollout) BeginSession() {
	now := time.Now()
	r.StartTime = now
	r.LastWriteTime = now
	r.GUID = now.Format("2006-01-02 15:04:05")
	r.AnalyticsCount = 0
	r.AnalyticsGUID = ""
	r.Generation = 0
	r.OverallFitnesses = nil
}

func (r *Rollout) ResetMetrics() {
	r.Metrics = NewMetrics()
}

func (r *Rollout) DynamicTemperature() float32 {
	if !r.Settings.Config.DynamicTemperature {
		return 0.1
	}
	if r.Settings.IsTraining {
		initialTemp := float32(1.5)
		minTemp := float32(0.1)
		halfGenerations := float32(r.Settings.Config.NumOfGenerations) / 2
		if halfGenerations <= 0 {
			return initialTemp
		}

		temp := initialTemp - (float32(r.Generation)/halfGenerations)*(initialTemp-minTemp)
		if temp < minTemp {
			temp = minTemp
		}
		return temp
	}
	return 0.1
}

func (r *Rollout) AllowStringOperations() bool {
	return !r.Settings.Flags.operationFlagsConfigured || r.Settings.Flags.AllowStringOperations
}

func (r *Rollout) AllowMathOperations() bool {
	return !r.Settings.Flags.operationFlagsConfigured || r.Settings.Flags.AllowMathOperations
}

func (r *Rollout) SafeAnalyticsGUID() string {
	if r.AnalyticsGUID != "" {
		return r.AnalyticsGUID
	}
	guid := strings.ReplaceAll(r.GUID, "/", "-")
	guid = strings.ReplaceAll(guid, ":", "-")
	r.AnalyticsGUID = guid
	return guid
}

func ResolveDataRoot(override string) (AppPaths, error) {
	if override != "" {
		root, err := filepath.Abs(override)
		if err != nil {
			return AppPaths{}, err
		}
		if err := validateDataRoot(root); err != nil {
			return AppPaths{}, err
		}
		return AppPaths{Root: root}, nil
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, "data"),
			filepath.Join(exeDir, "..", "data"),
		}
		for _, candidate := range candidates {
			root, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			if err := validateDataRoot(root); err == nil {
				return AppPaths{Root: root}, nil
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		root := filepath.Join(cwd, "data")
		if abs, err := filepath.Abs(root); err == nil {
			if err := validateDataRoot(abs); err == nil {
				return AppPaths{Root: abs}, nil
			}
		}
	}

	return AppPaths{}, fmt.Errorf("could not locate data directory (expected config/ under data/); use --data-dir")
}

func validateDataRoot(root string) error {
	info, err := os.Stat(filepath.Join(root, "config"))
	if err != nil {
		return fmt.Errorf("data directory %q has no config subdirectory: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("data directory %q config path is not a directory", root)
	}
	return nil
}

func parseDataDirFlag(args []string) (string, []string, error) {
	if len(args) < 1 {
		return "", args, nil
	}

	rest := append([]string(nil), args...)
	dataDir := ""
	for len(rest) > 2 && rest[1] == "--data-dir" {
		dataDir = rest[2]
		rest = append([]string{rest[0]}, rest[3:]...)
	}
	if len(rest) > 1 && rest[1] == "--data-dir" {
		return "", nil, fmt.Errorf("--data-dir requires a path")
	}
	return dataDir, rest, nil
}

func parseCPUFlag(args []string) (int, []string, error) {
	rest := make([]string, 0, len(args))
	cpus := 0

	for i := 0; i < len(args); i++ {
		if args[i] != "--cpus" {
			rest = append(rest, args[i])
			continue
		}
		if i+1 >= len(args) {
			return 0, nil, fmt.Errorf("--cpus requires a value")
		}
		n, err := strconv.Atoi(args[i+1])
		if err != nil || n < 1 {
			return 0, nil, fmt.Errorf("--cpus value must be a positive integer, got %q", args[i+1])
		}
		cpus = n
		i++
	}

	return cpus, rest, nil
}

// parseThresholdFlag extracts an optional "-t <comp-ratio>" pair from args.
// Returns nil when the flag is absent. Scans the whole slice, so the flag
// may appear before or after the path.
func parseThresholdFlag(args []string) (*float32, []string, error) {
	rest := make([]string, 0, len(args))
	var override *float32

	for i := 0; i < len(args); i++ {
		if args[i] != "-t" {
			rest = append(rest, args[i])
			continue
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("-t requires a value")
		}
		v, err := strconv.ParseFloat(args[i+1], 32)
		if err != nil {
			return nil, nil, fmt.Errorf("-t value %q is not a valid number: %w", args[i+1], err)
		}
		if v < 0 {
			return nil, nil, fmt.Errorf("-t value must be >= 0, got %v", v)
		}
		f := float32(v)
		override = &f
		i++ // consume the value
	}

	return override, rest, nil
}

// parseDeflateFitnessFlag extracts an optional --deflate-fitness boolean
// from args. Returns true when present. No value argument.
func parseDeflateFitnessFlag(args []string) (bool, []string, error) {
	rest := make([]string, 0, len(args))
	present := false

	for i := 0; i < len(args); i++ {
		if args[i] != "--deflate-fitness" {
			rest = append(rest, args[i])
			continue
		}
		present = true
	}

	return present, rest, nil
}

// parseCodecFlag extracts an optional --codec <name> pair from args. The
// name is matched case-insensitively. Returns nil when the flag is absent.
func parseCodecFlag(args []string) (*PackageCodec, []string, error) {
	rest := make([]string, 0, len(args))
	var result *PackageCodec

	for i := 0; i < len(args); i++ {
		if args[i] != "--codec" {
			rest = append(rest, args[i])
			continue
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("--codec requires a value")
		}
		name := strings.ToLower(strings.TrimSpace(args[i+1]))
		var codec PackageCodec
		switch name {
		case "raw", "none":
			codec = CodecRaw
		case "deflate", "flate":
			codec = CodecDeflate
		case "zstd":
			codec = CodecZstd
		case "xz", "lzma":
			codec = CodecXz
		default:
			return nil, nil, fmt.Errorf("unknown codec %q (want raw, deflate, zstd, or xz)", args[i+1])
		}
		result = &codec
		i++
	}

	return result, rest, nil
}

func FilterOperandLogitsByTrainingFlags(rollout *Rollout, logits []float32) {
	if rollout == nil {
		return
	}
	for i, operand := range operandClasses {
		if i >= len(logits) {
			break
		}
		if !rollout.OperationAllowed(operand) {
			logits[i] = float32(math.Inf(-1))
		}
	}
}

func AnyOperandAllowedByTrainingFlags(rollout *Rollout) bool {
	if rollout == nil {
		return true
	}
	for _, operand := range operandClasses {
		if rollout.OperationAllowed(operand) {
			return true
		}
	}
	return false
}

func (r *Rollout) OperationAllowed(operand string) bool {
	if IsStringOperation(operand) {
		return r.AllowStringOperations()
	}
	if IsMathOperation(operand) {
		return r.AllowMathOperations()
	}
	return true
}
