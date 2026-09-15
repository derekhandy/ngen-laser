package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ExhaustiveConfig struct {
	IterationLimit      int     `json:"iterationLimit"`
	NumOfGenerations    int     `json:"numOfGenerations"`
	PopulationSize      int     `json:"populationSize"`
	MutationPercent     float32 `json:"mutationPercent"`
	MutationStrength    float32 `json:"mutationStrength"`
	DynamicTemperature  bool    `json:"dynamicTemperature"`
	AllowStopping       bool    `json:"allowStopping"`
	Passes              int     `json:"passes"`
	InputSize           int     `json:"inputSize"`
	HiddenLayers        []int   `json:"hiddenLayers"`
	ElitePercent        float32 `json:"elitePercent"`
	MutatedElitePercent float32 `json:"mutatedElitePercent"`
	CrossoverPercent    float32 `json:"crossoverPercent"`
	NewInitPercent      float32 `json:"newInitPercent"`
}

type TrainingRubric struct {
	InvalidPenalty          float32 `json:"invalidPenalty"`
	ValidReward             float32 `json:"validReward"`
	ValidIndex              float32 `json:"validIndex"`
	ValidOperand            float32 `json:"validOperand"`
	RepeatComboPenalty      float32 `json:"repeatComboPenalty"`
	AttemptPenalty          float32 `json:"attemptPenalty"`
	GlobalImprovementReward float32 `json:"globalImprovementReward"`
	LengthReductionReward   float32 `json:"lengthReductionReward"`
	SizeGrowthPunishment    float32 `json:"SizeGrowthPunishment"`
	ImprovementStreakReward float32 `json:"improvementStreakReward"`
	FirstAttemptReward      float32 `json:"firstAttemptReward"`
	FirstOperandReward      float32 `json:"firstOperandReward"`
	FirstIndexUseReward     float32 `json:"firstIndexUseReward"`
	DiscoveryReward         float32 `json:"discoveryReward"`
	TunnelPunishment        float32 `json:"tunnelPunishment"`
	EarlyStopPenalty        float32 `json:"earlyStopPenalty"`
	ValidStopReward         float32 `json:"validStopReward"`
}

type TrainingFlags struct {
	WriteGenerationAnalytics        bool    `json:"writeGenerationAnalytics"`
	GenerationBasedNetworkWrite     bool    `json:"generationBasedNetworkWrite"`
	WriteNetworksGenerationInterval int     `json:"writeNetworksGenerationInterval"`
	TimeBasedNetworkWrite           bool    `json:"timeBasedNetworkWrite"`
	WriteNetworksIntervalTime       int     `json:"writeNetworksIntervalTime"`
	WarmupMemory                    bool    `json:"warmupMemory"`
	DebugMode                       bool    `json:"debugMode"`
	ThresholdCompToWrite            float32 `json:"thresholdCompToWrite"`
	EndTrainingOnThreshold          bool    `json:"endTrainingOnThreshold"`
	AllowStringOperations           bool    `json:"allowStringOperations"`
	AllowMathOperations             bool    `json:"allowMathOperations"`
	operationFlagsConfigured        bool
}

type RolloutSettings struct {
	IsTraining       bool
	IsGUI            bool
	InstructionPaths []string
	Config           ExhaustiveConfig
	Rubric           TrainingRubric
	Flags            TrainingFlags
	Monitor          *ResourceMonitor
}

type RolloutMetrics struct {
	ItemIndex    int
	ItemTotal    int
	TotalComp    float32
	PassIndex    int
	OriginalSize int
	PackagedSize int
	PassDuration time.Duration
}

type LinkedContainerData struct {
	Id    string
	Paths []string
}

func LoadDefaultExhaustiveConfig() ExhaustiveConfig {
	return ExhaustiveConfig{
		IterationLimit:      32,
		NumOfGenerations:    1,
		PopulationSize:      150,
		MutationPercent:     0.05,
		MutationStrength:    0.08,
		DynamicTemperature:  true,
		AllowStopping:       true,
		Passes:              10,
		InputSize:           defaultInputSize,
		HiddenLayers:        append([]int(nil), defaultHiddenLayers...),
		ElitePercent:        0.1,
		MutatedElitePercent: 0.6,
		CrossoverPercent:    0.15,
		NewInitPercent:      0.15,
	}
}

func LoadDefaultTrainingFlags() TrainingFlags {
	return TrainingFlags{
		WriteGenerationAnalytics:        false,
		GenerationBasedNetworkWrite:     true,
		WriteNetworksGenerationInterval: 1,
		TimeBasedNetworkWrite:           true,
		WriteNetworksIntervalTime:       300,
		WarmupMemory:                    true,
		DebugMode:                       false,
		ThresholdCompToWrite:            0,
		EndTrainingOnThreshold:          false,
		AllowStringOperations:           true,
		AllowMathOperations:             true,
		operationFlagsConfigured:        true,
	}
}

func LoadExhaustiveConfig(path string) (ExhaustiveConfig, error) {
	if path == "" {
		return LoadDefaultExhaustiveConfig(), fmt.Errorf("[!] traing-config.json path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ExhaustiveConfig{}, err
	}
	var cfg ExhaustiveConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ExhaustiveConfig{}, err
	}

	return cfg, nil
}

func LoadTrainingFlags(path string) (TrainingFlags, error) {
	if path == "" {
		return LoadDefaultTrainingFlags(), fmt.Errorf("[!] training-flags.json path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return TrainingFlags{}, err
	}
	flags := LoadDefaultTrainingFlags()
	if err := json.Unmarshal(data, &flags); err != nil {
		return TrainingFlags{}, err
	}
	flags.operationFlagsConfigured = true

	return flags, nil
}

func LoadRubric(path string) (TrainingRubric, error) {
	if path == "" {
		return TrainingRubric{}, fmt.Errorf("[!] environment-rubric.json path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return TrainingRubric{}, err
	}
	var rubric TrainingRubric
	if err := json.Unmarshal(data, &rubric); err != nil {
		return TrainingRubric{}, err
	}

	return rubric, nil
}

func NewMetrics() RolloutMetrics {
	_metrics := RolloutMetrics{
		ItemIndex:    0,
		ItemTotal:    0,
		TotalComp:    0.0,
		PassIndex:    0,
		OriginalSize: 0,
		PackagedSize: 0,
		PassDuration: time.Duration(0),
	}

	return _metrics
}

func PrepareTraining(rollout *Rollout) ([]*Network, *Tree, error) {
	rollout.BeginSession()

	if err := LoadConfigs(rollout, []int{1, 1, 1}); err != nil {
		return nil, nil, err
	}

	tree, err := PrepareTree(rollout.Settings.InstructionPaths)
	if err != nil {
		return nil, nil, err
	}

	population, err := InitializeAndLoadNetworks(rollout)
	if err != nil {
		return nil, nil, err
	}

	if rollout.Settings.Flags.WarmupMemory {
		rollout.CurrentPassthroughLimit = 1
	} else {
		rollout.CurrentPassthroughLimit = rollout.Settings.Config.Passes
	}

	return population, tree, nil
}

func PreparePackaging(rollout *Rollout) ([]*Network, *Tree, error) {
	rollout.BeginSession()

	if err := LoadConfigs(rollout, []int{1, 0, 1}); err != nil {
		return nil, nil, err
	}

	tree, err := PrepareTree(rollout.Settings.InstructionPaths)
	if err != nil {
		return nil, nil, err
	}

	population, err := InitializeAndLoadNetworks(rollout)
	if err != nil {
		return nil, nil, err
	}

	if rollout.Settings.Flags.WarmupMemory {
		rollout.CurrentPassthroughLimit = 1
	} else {
		rollout.CurrentPassthroughLimit = rollout.Settings.Config.Passes
	}

	return population, tree, nil
}

func LoadConfigs(rollout *Rollout, load []int) error {
	if load[0] == 1 {
		LogLoading("Loading config")

		cfgType := "training-config.json"
		if !rollout.Settings.IsTraining {
			cfgType = "solving-config.json"
		}

		_cfg, err := LoadExhaustiveConfig(rollout.Paths.Config(cfgType))
		if err != nil {
			return fmt.Errorf("[!] could not load %s: %v", cfgType, err)
		}

		if !ConfigValidationPass(_cfg) {
			return fmt.Errorf("[!] error in configuration: %s", cfgType)
		}

		rollout.Settings.Config = _cfg

		PrintConfirm("done")
	}

	if load[1] == 1 {
		LogLoading("Loading rubric")

		_rubric, err := LoadRubric(rollout.Paths.Config("environment-rubric.json"))
		if err != nil {
			LogError("loading environment-rubric.json", err)
			return fmt.Errorf("[!] could not load environment-rubric.json")
		}

		rollout.Settings.Rubric = _rubric

		PrintConfirm("done")
	}

	if load[2] == 1 {
		LogLoading("Loading training flags")

		_flags, err := LoadTrainingFlags(rollout.Paths.Config("training-flags.json"))
		if err != nil {
			return fmt.Errorf("[!] Could not load training-flags.json: %v\n", err)
		}

		if rollout.ThresholdCompOverride != nil {
			_flags.ThresholdCompToWrite = *rollout.ThresholdCompOverride
			LogInfo("thresholdCompToWrite overridden to %.4f via -t", *rollout.ThresholdCompOverride)
		}

		rollout.Settings.Flags = _flags

		PrintConfirm("done")
	}

	return nil
}

func InitializeAndLoadNetworks(rollout *Rollout) ([]*Network, error) {
	SetDefaultArchitecture(rollout.Settings.Config.InputSize, rollout.Settings.Config.HiddenLayers)

	if rollout.Settings.Flags.DebugMode {
		LogInfo("debug mode: setting population to 1")
		_population := InitializePopulation(defaultInputSize, defaultHiddenLayers, 1)
		if saved, loadErr := LoadNetwork(rollout.Paths.Weights("elite_0.json")); loadErr != nil {
			PrintConfirm("pass")
			LogInfo("elite_0.json not found; using a new network")
		} else {
			PrintConfirm("done")
			LogLoading(fmt.Sprintf("Initializing %d population slots with prime network", maxSavedElites))
			InjectPreviousBest(1, _population, &saved, rollout.Settings.Config)
			PrintConfirm("done")
		}
		return _population, nil
	}
	LogLoading("Initializing empty population")

	population := InitializePopulation(defaultInputSize, defaultHiddenLayers, rollout.Settings.Config.PopulationSize)

	PrintConfirm("done")

	LogLoading("Loading existing prime network")

	if saved, loadErr := LoadNetwork(rollout.Paths.Weights("elite_0.json")); loadErr != nil {
		PrintConfirm("pass")
		LogInfo("elite_0.json not found; using a new network")
	} else {
		PrintConfirm("done")
		LogLoading(fmt.Sprintf("Initializing %d population slots with prime network", maxSavedElites))
		InjectPreviousBest(rollout.Settings.Config.PopulationSize, population, &saved, rollout.Settings.Config)
		PrintConfirm("done")
	}

	loadingErrors := 0
	LogLoading("Loading additional saved networks into elite population slots")
	for i := 0; i < maxSavedElites && i < rollout.Settings.Config.PopulationSize; i++ {
		saved, loadErr := LoadNetwork(rollout.Paths.Weights("elite_" + strconv.Itoa(i) + ".json"))
		if loadErr == nil {
			population[i] = CloneNetwork(&saved)
		} else {
			loadingErrors++
		}
	}

	PrintConfirm("done")

	if loadingErrors > 0 {
		LogInfo("%d elite slots were filled with prime clones or new networks", loadingErrors)
	}

	return population, nil
}

func PrepareTree(instructionPaths []string) (*Tree, error) {
	LogLoading("Loading and converting instruction roots")
	if len(instructionPaths) == 0 {
		PrintConfirm("fail")
		return nil, fmt.Errorf("no instruction roots configured")
	}

	var merged *Tree
	for rootIndex, root := range instructionPaths {
		if strings.TrimSpace(root) == "" {
			PrintConfirm("fail")
			return nil, fmt.Errorf("instruction root %d is empty", rootIndex)
		}
		part, err := DirectoryToContainers(root)
		if err != nil {
			if merged != nil {
				for _, tempPath := range merged.TempPaths {
					_ = os.RemoveAll(tempPath)
				}
			}
			PrintConfirm("fail")
			return nil, fmt.Errorf("prepare instruction root %q: %w", root, err)
		}
		if part == nil {
			PrintConfirm("fail")
			return nil, fmt.Errorf("instruction root %q produced no tree", root)
		}

		if len(instructionPaths) > 1 {
			rootLabel := fmt.Sprintf("%d-%s", rootIndex, filepath.Base(filepath.Clean(root)))
			for i := range part.Containers {
				part.Containers[i].RelativePath = filepath.ToSlash(filepath.Join(rootLabel, part.Containers[i].RelativePath))
			}
		}

		if merged == nil {
			merged = part
			continue
		}

		merged.Containers = append(merged.Containers, part.Containers...)
		merged.TempPaths = append(merged.TempPaths, part.TempPaths...)
	}

	if merged == nil || len(merged.Containers) == 0 {
		PrintConfirm("fail")
		return nil, fmt.Errorf("instruction roots produced no files")
	}
	if len(instructionPaths) > 1 {
		merged.Name = "laser"
	}
	PrintConfirm("done")
	return merged, nil
}

func ConfigValidationPass(cfg ExhaustiveConfig) bool {
	if cfg.PopulationSize <= 0 || cfg.Passes <= 0 || cfg.NumOfGenerations <= 0 {
		return false
	}
	for _, hiddenSize := range cfg.HiddenLayers {
		if hiddenSize <= 0 {
			return false
		}
	}
	return true
}

func PrepValidationPass(populationLength int, containersLength int, cfg ExhaustiveConfig) bool {
	return populationLength > 0 && (populationLength == cfg.PopulationSize) && containersLength > 0
}
