package main

import (
	"fmt"
	"math/rand"
)

type StepMetrics struct {
	originalSize      int
	previousSize      int
	newSize           int
	packedImprovement bool
	globalSizeReduced bool
	firstOperandUse   bool
	sizeReduction     int
	attempts          int
	interpreted       string
	improvementStreak int
	chainLength       int
	operand           string
	index             int
}

type ChainStep struct {
	Index   int
	Operand string
	Params  map[string]interface{}
}

type CompressionEnvironment struct {
	rollout                *Rollout
	commands               *ICommands
	originalInstructions   string
	chainRewardGranted     bool
	allowOperand           bool
	current                string
	bestString             string
	bestPackedSize         int
	attemptedSteps         int
	successfulSteps        int
	successfulImprovements int
	improvementStreak      int
	invalidComboCounts     map[string]map[int]int
	indexUseCounts         map[int]int
	operandUseCounts       map[string]int
	hasUsedOperand         map[string]bool
	seenIndexLengths       map[string]map[int]bool
	indexCoverageBuckets   []bool
	coverageBucketsFrozen  []bool
	indexUseMemory         map[int]int
	lastImprovementIdx     int
	currentChain           []ChainStep
	bestChain              []ChainStep
	memory                 *MemoryController
	fitness                float32
}

func NewCompressionEnvironment(original string, rollout *Rollout) *CompressionEnvironment {

	commands := NewICommands()
	interpreted := commands.ReturnInterpret(original, 0, &[]IndexEntry{})

	return &CompressionEnvironment{
		rollout:              rollout,
		commands:             commands,
		originalInstructions: original,
		current:              interpreted,
		bestString:           interpreted,
		bestPackedSize:       PackedInstructionSize(interpreted),
		invalidComboCounts:   make(map[string]map[int]int),
		indexUseCounts:       make(map[int]int),
		operandUseCounts:     make(map[string]int),
		hasUsedOperand:       make(map[string]bool),
		seenIndexLengths:     make(map[string]map[int]bool),
		indexCoverageBuckets: make([]bool, MaxInt(4, len(interpreted)/12)),
		indexUseMemory:       make(map[int]int),
		lastImprovementIdx:   -1,
		currentChain:         nil,
		bestChain:            nil,
		allowOperand:         true,
		memory:               nil,
		fitness:              0.0,
	}
}

func (env *CompressionEnvironment) Reset() string {
	env.current = env.originalInstructions
	if env.bestString == "" {
		env.bestString = env.current
	}
	if env.bestPackedSize <= 0 {
		env.bestPackedSize = PackedInstructionSize(env.bestString)
	}

	if env.currentChain != nil {
		env.currentChain = env.currentChain[:0]
	} else {
		env.currentChain = make([]ChainStep, 0, 100)
	}

	clear(env.indexUseCounts)
	clear(env.operandUseCounts)
	clear(env.indexUseMemory)

	for k := range env.seenIndexLengths {
		delete(env.seenIndexLengths, k)
	}

	if env.indexCoverageBuckets != nil {
		clear(env.indexCoverageBuckets)
	}
	env.coverageBucketsFrozen = nil

	env.ClearMetrics()
	env.successfulSteps = 0
	env.lastImprovementIdx = -1
	env.fitness = 0.0
	env.chainRewardGranted = false

	return env.current
}

func (env *CompressionEnvironment) ClearMetrics() {
	env.invalidComboCounts = make(map[string]map[int]int)
	env.attemptedSteps = 0
	env.successfulSteps = 0
	env.successfulImprovements = 0
	env.operandUseCounts = make(map[string]int)
	env.hasUsedOperand = make(map[string]bool)
	env.improvementStreak = 0
	env.chainRewardGranted = false
	env.lastImprovementIdx = -1
	env.fitness = 0.0
	env.currentChain = nil
	if len(env.coverageBucketsFrozen) > 0 {
		env.indexCoverageBuckets = append([]bool(nil), env.coverageBucketsFrozen...)
	} else if len(env.indexCoverageBuckets) == 0 {
		buckets := MaxInt(4, len(env.current)/12)
		env.indexCoverageBuckets = make([]bool, buckets)
	}
}

func (env *CompressionEnvironment) StepWithManual(idx int, operand string, params map[string]interface{}) (bool, bool) {
	if len(env.current) > maxStringLength {
		return false, true
	}

	env.RecordAttempt(idx)

	prevString := env.current
	prevSize := PackedInstructionSize(prevString)

	var interpreted string
	newSize := prevSize

	stop := false
	invalid := false
	validAction := false
	globalSizeReduced := false

	if requestedStop, ok := params["stop"].(bool); ok && requestedStop {
		stop = true
	}

	if !stop {
		next, valid := env.ApplyCandidate(idx, operand, CloneParams(params))

		if !valid {
			invalid = true
		} else {
			interpreted = env.commands.ReturnInterpret(next, 0, &[]IndexEntry{})

			if interpreted == "" {
				invalid = true
			} else if !env.commands.Validate(interpreted, env.originalInstructions) {
				invalid = true
			} else {
				validAction = true
				newSize = PackedInstructionSize(interpreted)

				if interpreted != env.current {
					env.current = interpreted
				}
			}
		}

	}

	if validAction && !invalid {
		env.currentChain = append(env.currentChain, ChainStep{
			Index:   idx,
			Operand: operand,
			Params:  CloneParams(params),
		})

		env.CommitState(idx, operand)

		globalSizeReduced = newSize < env.bestPackedSize

		if globalSizeReduced {
			env.bestString = env.current
			env.bestPackedSize = newSize
			env.bestChain = append([]ChainStep(nil), env.currentChain...)
			env.CommitBest()
		}
	}

	if invalid {
		env.InvalidateCombo(idx, operand)
	}

	env.fitness = 0

	return !invalid, stop
}

func (env *CompressionEnvironment) Step(output NetworkOutput, iterationLimit int, r *rand.Rand) (bool, bool) {
	if len(env.current) > maxStringLength {
		env.fitness = -5
		DebugPrint(env.rollout, fmt.Sprintf("NETWORK STEP\n[!]LENGTH [%d] LONGER THAN MAXSTRINGLENGTH [%d]", len(env.current), maxStringLength), false)
		return false, true
	}

	env.attemptedSteps++

	score := env.rollout.Settings.Rubric.AttemptPenalty
	scaledInvalidPenalty := env.rollout.Settings.Rubric.InvalidPenalty / float32(iterationLimit)

	idx, operand, params := DecodeDecision(output, len(env.current), r, env.rollout)

	env.RecordAttempt(idx)

	prevString := env.current
	prevSize := PackedInstructionSize(prevString)

	interpreted := prevString
	newSize := prevSize

	stop := false
	invalid := false
	validAction := false
	firstOpUse := false
	firstIndexUse := false
	globalSizeReduced := false
	packedImprovement := false
	packedRegression := false
	packedWorse := false

	if requestedStop, ok := params["stop"].(bool); ok && requestedStop {
		stop = true
	}

	if !stop {
		next, valid := env.ApplyCandidate(idx, operand, CloneParams(params))

		if !valid {
			invalid = true
		} else {
			interpreted = env.commands.ReturnInterpret(next, 0, &[]IndexEntry{})

			if interpreted == "" {
				invalid = true
			} else if !env.commands.Validate(interpreted, env.originalInstructions) {
				invalid = true
			} else {
				validAction = true
				newSize = PackedInstructionSize(interpreted)

				if interpreted != env.current {
					env.current = interpreted
				}
			}
		}

	}

	if validAction && !invalid {
		env.currentChain = append(env.currentChain, ChainStep{
			Index:   idx,
			Operand: operand,
			Params:  CloneParams(params),
		})

		firstOpUse = env.IsFirstUse(operand)
		firstIndexUse = env.IsFirstIndexBucket(idx, len(interpreted))

		env.CommitState(idx, operand)

		globalSizeReduced = newSize < env.bestPackedSize
		packedImprovement = newSize < prevSize
		packedRegression = newSize > prevSize
		packedWorse = newSize > prevSize

		if globalSizeReduced {
			env.bestString = env.current
			env.bestPackedSize = newSize
			env.bestChain = append([]ChainStep(nil), env.currentChain...)
			env.CommitBest()
		}
	}

	if invalid {
		env.improvementStreak = 0

		env.InvalidateCombo(idx, operand)

		score += scaledInvalidPenalty
		score += float32(env.invalidComboCounts[operand][idx]) *
			env.rollout.Settings.Rubric.RepeatComboPenalty
	} else if validAction {
		env.successfulSteps++

		if packedImprovement {
			env.successfulImprovements++
			env.improvementStreak++
			metrics := StepMetrics{
				originalSize:      PackedInstructionSize(env.originalInstructions),
				previousSize:      prevSize,
				newSize:           newSize,
				packedImprovement: true,
				globalSizeReduced: globalSizeReduced,
				firstOperandUse:   firstOpUse,
				sizeReduction:     prevSize - newSize,
				attempts:          env.attemptedSteps,
				interpreted:       interpreted,
				improvementStreak: env.improvementStreak,
				chainLength:       len(env.currentChain),
				operand:           operand,
				index:             idx,
			}

			score += env.CalculateReward(metrics)

		} else {
			if env.improvementStreak > 0 {
				env.improvementStreak--
			}
		}
	}

	if packedRegression {
		growthPercentage := float32(0.0)
		if PackedInstructionSize(env.originalInstructions) > 0 {
			growthPercentage = float32(newSize) / float32(PackedInstructionSize(env.originalInstructions))
		}
		score += growthPercentage * env.rollout.Settings.Rubric.SizeGrowthPunishment
	}

	if validAction && !packedWorse && firstOpUse {
		score += env.rollout.Settings.Rubric.FirstOperandReward
	} else if validAction && (env.operandUseCounts[operand] > 5 ||
		env.indexUseMemory[idx] > 5) {
		score -= env.rollout.Settings.Rubric.TunnelPunishment
	}

	if validAction && !packedWorse && firstIndexUse {
		score += env.rollout.Settings.Rubric.FirstIndexUseReward
	}

	if globalSizeReduced && validAction {
		DebugPrint(env.rollout, fmt.Sprintf("NETWORK STEP\nINDEX\t%d\nOPERAND\t%s\nPARAMS\t%v\nIMPROVEMENT\t%v\nSCORE\t%.2f\n", idx, operand, params, globalSizeReduced, score), false)
	}

	env.fitness += score

	return !invalid, stop
}

func (env *CompressionEnvironment) CalculateReward(metrics StepMetrics) float32 {
	sizeReduction := metrics.PacketSizeReduction()
	if !metrics.packedImprovement || sizeReduction <= 0 {
		return 0
	}

	reward := env.rollout.Settings.Rubric.ValidReward

	if metrics.globalSizeReduced {
		reward += env.rollout.Settings.Rubric.GlobalImprovementReward

		if env.seenIndexLengths == nil {
			env.seenIndexLengths = make(map[string]map[int]bool)
		}
		if env.seenIndexLengths[metrics.operand] == nil {
			env.seenIndexLengths[metrics.operand] = make(map[int]bool)
		}
		if !env.seenIndexLengths[metrics.operand][metrics.index] {
			reward += env.rollout.Settings.Rubric.DiscoveryReward
			env.seenIndexLengths[metrics.operand][metrics.index] = true
		}
	}

	if metrics.attempts == 1 {
		reward += env.rollout.Settings.Rubric.FirstAttemptReward
	}

	reward += float32(sizeReduction) * env.rollout.Settings.Rubric.LengthReductionReward

	reductionPercentage := float32(0.0)
	if metrics.originalSize > 0 {
		reductionPercentage = float32(sizeReduction) / float32(metrics.originalSize)
	}

	reward += reductionPercentage * env.rollout.Settings.Rubric.GlobalImprovementReward

	if metrics.improvementStreak > 1 {
		reward += float32(metrics.improvementStreak-1) * env.rollout.Settings.Rubric.ImprovementStreakReward
	}

	return reward
}

func (env *CompressionEnvironment) ApplyCandidate(idx int, operand string, params map[string]interface{}) (string, bool) {

	next, err := ApplyOperation(env.rollout, env.current, idx, operand, CloneParams(params))
	if err != nil {
		return next, false
	}

	if !env.commands.Validate(next, env.originalInstructions) {
		return next, false
	}
	return next, true
}

func (env *CompressionEnvironment) OperandCoverageRatio() float32 {
	unique := 0
	for _, count := range env.operandUseCounts {
		if count > 0 {
			unique++
		}
	}
	return float32(unique) / float32(len(operandClasses))
}

func (env *CompressionEnvironment) CommitBest() {
	if len(env.indexCoverageBuckets) == 0 {
		return
	}

	if len(env.coverageBucketsFrozen) != len(env.indexCoverageBuckets) {
		env.coverageBucketsFrozen = make([]bool, len(env.indexCoverageBuckets))
	}

	copy(env.coverageBucketsFrozen, env.indexCoverageBuckets)
}

func (env *CompressionEnvironment) CommitState(idx int, operand string) {
	if env.indexUseCounts == nil {
		env.indexUseCounts = make(map[int]int)
	}
	if idx >= 0 {
		env.indexUseCounts[idx]++
	}
	env.operandUseCounts[operand]++

	if len(env.indexCoverageBuckets) > 0 {
		bucket := env.IndexCoverageBucket(idx, len(env.current))

		env.indexCoverageBuckets[bucket] = true
	}

	env.lastImprovementIdx = idx
}

func (env *CompressionEnvironment) IsFirstIndexBucket(idx int, instructionLength int) bool {
	if len(env.indexCoverageBuckets) == 0 {
		return false
	}
	return !env.indexCoverageBuckets[env.IndexCoverageBucket(idx, instructionLength)]
}

func (env *CompressionEnvironment) IndexCoverageBucket(idx int, instructionLength int) int {
	if len(env.indexCoverageBuckets) == 0 {
		return 0
	}
	if instructionLength <= 0 {
		return 0
	}
	bucket := int(float32(idx) / float32(instructionLength) * float32(len(env.indexCoverageBuckets)))
	return ClampInt(bucket, 0, len(env.indexCoverageBuckets)-1)
}

func (env *CompressionEnvironment) RecordAttempt(idx int) {
	if env.indexUseMemory == nil {
		env.indexUseMemory = make(map[int]int)
	}
	if idx >= 0 {
		env.indexUseMemory[idx]++
	}
}

func (env *CompressionEnvironment) IsFirstUse(operand string) bool {
	if !env.hasUsedOperand[operand] {
		env.hasUsedOperand[operand] = true
		return true
	}

	return false
}

func (env *CompressionEnvironment) InvalidateCombo(idx int, operand string) {
	if env.invalidComboCounts == nil {
		env.invalidComboCounts = make(map[string]map[int]int)
	}
	if env.invalidComboCounts[operand] == nil {
		env.invalidComboCounts[operand] = make(map[int]int)
	}
	env.invalidComboCounts[operand][idx] = env.invalidComboCounts[operand][idx] + 1
}

func (env *CompressionEnvironment) Clone() *CompressionEnvironment {
	clone := *env

	clone.commands = NewICommands()

	if env.indexUseCounts != nil {
		clone.indexUseCounts = make(map[int]int)
		for k, v := range env.indexUseCounts {
			clone.indexUseCounts[k] = v
		}
	}

	if env.operandUseCounts != nil {
		clone.operandUseCounts = make(map[string]int)
		for k, v := range env.operandUseCounts {
			clone.operandUseCounts[k] = v
		}
	}

	if env.indexCoverageBuckets != nil {
		clone.indexCoverageBuckets = make([]bool, len(env.indexCoverageBuckets))
		copy(clone.indexCoverageBuckets, env.indexCoverageBuckets)
	}

	if env.coverageBucketsFrozen != nil {
		clone.coverageBucketsFrozen = make([]bool, len(env.coverageBucketsFrozen))
		copy(clone.coverageBucketsFrozen, env.coverageBucketsFrozen)
	}

	if env.indexUseMemory != nil {
		clone.indexUseMemory = make(map[int]int)
		for k, v := range env.indexUseMemory {
			clone.indexUseMemory[k] = v
		}
	}

	if len(env.bestChain) > 0 {
		clone.bestChain = make([]ChainStep, len(env.bestChain))
		for i, step := range env.bestChain {
			clone.bestChain[i] = ChainStep{
				Index:   step.Index,
				Operand: step.Operand,
				Params:  CloneParams(step.Params),
			}
		}
	}

	if len(env.currentChain) > 0 {
		clone.currentChain = make([]ChainStep, len(env.currentChain))
		for i, step := range env.currentChain {
			clone.currentChain[i] = ChainStep{
				Index:   step.Index,
				Operand: step.Operand,
				Params:  CloneParams(step.Params),
			}
		}
	}

	clone.seenIndexLengths = make(map[string]map[int]bool, len(env.seenIndexLengths))
	for k, v := range env.seenIndexLengths {
		innerMap := make(map[int]bool, len(v))
		for key, val := range v {
			innerMap[key] = val
		}
		clone.seenIndexLengths[k] = innerMap
	}

	return &clone
}

func (env *CompressionEnvironment) BestChain() []ChainStep {
	if len(env.bestChain) == 0 {
		return nil
	}
	chain := make([]ChainStep, len(env.bestChain))
	for i, step := range env.bestChain {
		chain[i] = ChainStep{
			Index:   step.Index,
			Operand: step.Operand,
			Params:  CloneParams(step.Params),
		}
	}
	return chain
}
