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
	"strconv"
	"strings"
	"sync"
	"time"
)

type ICommands struct{}

var currentString = ""

func NewICommands() *ICommands {
	return &ICommands{}
}

type StringCache struct {
	sync.RWMutex
	Lookup map[string]bool
}

func GetCurrentString() string {
	return currentString
}

func ComputeCompression(rollout *Rollout, args string) error {
	params := strings.Split(args, ",")
	if len(params) != 2 {
		return fmt.Errorf("expected <var-max>,<iteration-max>")
	}

	varMax, err := strconv.Atoi(strings.TrimSpace(params[0]))
	if err != nil {
		return fmt.Errorf("invalid var-max %q: %w", params[0], err)
	}

	iterationMax, err := strconv.Atoi(strings.TrimSpace(params[1]))
	if err != nil {
		return fmt.Errorf("invalid iteration-max %q: %w", params[1], err)
	}

	if varMax < 1 || iterationMax < 1 {
		return fmt.Errorf("var-max and iteration-max must be greater than zero")
	}

	rollout.ResetMetrics()
	rollout.Settings.IsTraining = false

	ClearLogs(rollout)

	LogLoading("Loading config")

	cfg, err := LoadExhaustiveConfig(rollout.Paths.Config("solving-config.json"))
	if err != nil {
		PrintConfirm("fail")
		return fmt.Errorf("could not load solving-config.json: %v", err)
	}

	if !ConfigValidationPass(cfg) {
		PrintConfirm("fail")
		return fmt.Errorf("error in solving configuration: solving-config.json")
	}

	PrintConfirm("pass")

	LogLoading("Loading and converting directory to tree")

	tree, err := PrepareTree(rollout.Settings.InstructionPaths)
	if err != nil {
		PrintConfirm("fail")
		return fmt.Errorf("error preparing tree: %v", err)
	}

	PrintConfirm("pass")

	i, err := ComputeLzrs(rollout, tree, cfg, params)
	if i != 0 {
		return fmt.Errorf("error in ComputeLzrs(): %v", err)
	}

	return nil
}

func ComputeLzrs(rollout *Rollout, tree *Tree, cfg ExhaustiveConfig, params []string) (int, error) {
	sizes := []int{0, 0}

	LogLoading("Parsing compute packing depth parameters")

	varMax, err := strconv.Atoi(params[0])
	if err != nil {
		PrintConfirm("fail")
		return 1, fmt.Errorf("error parsing operand magnitude depth: %v", err)
	}

	iterationMax, err := strconv.Atoi(params[1])
	if err != nil {
		PrintConfirm("fail")
		return 1, fmt.Errorf("error parsing iteration max: %v", err)
	}

	PrintConfirm("pass")

	fmt.Print("\n\nSettings:\n\n")

	fmt.Printf("\tIndex Depth\t\t%d\n", varMax)
	fmt.Printf("\tIteration Max\t%d\n", iterationMax)

	fmt.Print("\n\nWaiting 5 seconds to start...")

	time.Sleep(5 * time.Second)

	ClearLogs(rollout)

	for _, container := range tree.Containers {
		rollout.Metrics.ItemTotal += len(container.Lzrs)
	}

	for containerIndex := range tree.Containers {
		container := &tree.Containers[containerIndex]
		for _, lzr := range container.Lzrs {
			rollout.Metrics.ItemIndex++

			PrintGUICodes(rollout)

			inst, err := os.ReadFile(lzr)
			if err != nil {
				return 1, err
			}

			env := NewCompressionEnvironment(string(inst), rollout)

			ClearLogs(rollout)
			fmt.Printf("Computing part %d/%d... %.2f%%", rollout.Metrics.ItemIndex, rollout.Metrics.ItemTotal, float32(rollout.Metrics.ItemIndex-1)/float32(rollout.Metrics.ItemTotal)*100.0)

			bestString, _sizes, err := ComputeInstructions(env, params, sizes, varMax, iterationMax)
			if err != nil {
				PrintConfirm("fail")
				return 1, fmt.Errorf("error in ComputeInstructions(): %v", err)
			}

			PrintConfirm("pass")

			sizes = _sizes

			if err := WriteNetworkOutput(container, bestString, lzr); err != nil {
				return 1, err
			}
			rollout.Metrics.PassIndex = 0
		}
	}

	if err := BuildPackage(tree); err != nil {
		return 1, fmt.Errorf("build package: %w", err)
	}
	fmt.Printf("\n\nBuilt package with comp%%\t%.2f\nOriginal : Packaged Size\t%d : %d", (float32(sizes[1]) / float32(sizes[0])), sizes[0], sizes[1])
	time.Sleep(5 * time.Second)

	return 0, nil
}

func ComputeInstructions(env *CompressionEnvironment, params []string, sizes []int, varMax int, iterationMax int) (string, []int, error) {
	bestString := env.originalInstructions
	bestSize := PackedInstructionSize(bestString)
	numWorkers := runtime.NumCPU()

	for i := 0; i < iterationMax; i++ {
		startTime := time.Now()

		operands := AllOperandSpellingsForLength(env.rollout, len(bestString), varMax)

		var mu sync.Mutex
		iterationBestString := bestString
		iterationBestSize := bestSize
		foundImprovement := false

		var wg sync.WaitGroup
		jobs := make(chan int, numWorkers*2)

		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c := NewICommands()

				for j := range jobs {
					prefix := bestString[0:j]
					suffix := bestString[j:]

					localBestSize := bestSize
					localBestString := ""

					for _, op := range operands {
						inserted := prefix + "!" + op + suffix

						interpreted := c.ReturnInterpret(inserted, 0, &[]IndexEntry{})

						if interpreted == "" || interpreted == "+" || interpreted == bestString {
							continue
						}

						currentPackedSize := PackedInstructionSize(interpreted)
						if currentPackedSize >= localBestSize {
							continue
						}

						if !ValidateStrings(bestString, inserted, true) {
							continue
						}

						localBestSize = currentPackedSize
						localBestString = interpreted
					}

					if localBestString != "" {
						mu.Lock()
						if localBestSize < iterationBestSize {
							iterationBestSize = localBestSize
							iterationBestString = localBestString
							foundImprovement = true
						}
						mu.Unlock()
					}
				}
			}()
		}

		go func() {
			for j := 0; j < len(bestString); j++ {
				jobs <- j
			}
			close(jobs)
		}()

		wg.Wait()

		if foundImprovement {
			bestString = iterationBestString
			bestSize = iterationBestSize
			fmt.Printf("\rIter %d | Size: %d | Time: %v    ", i, bestSize, time.Since(startTime))
		} else {
			break
		}
	}

	sizes[0] += PackedInstructionSize(env.originalInstructions)
	sizes[1] += bestSize
	return bestString, sizes, nil
}

func AllOperandSpellingsForLength(rollout *Rollout, instructionLength int, values ...int) []string {
	singleOperands := []string{
		"h", "k", "n", "b", "g", "l",
	}
	directionalOperands := []string{
		"tu", "td", "tl", "tr", "tf", "tb",
		"fu", "fd", "fl", "fr", "ff", "fb",
	}
	magnitudeOperands := []string{
		"x1", "x2", "x3", "x4",
		"y1", "y2", "y3", "y4",
		"z1", "z2", "z3", "z4",
		// add "o1-9" if implemented
	}
	mirrorOperands := []string{
		"rx", "ry", "rz",
	}
	mathOperands := []string{
		"m2", "d2",
	}

	varMax := variableMaxMagnitude
	if len(values) > 0 {
		varMax = values[0]
	}

	indexLengths := IndexLengthCandidatesForInstruction(instructionLength, varMax)
	indexOperands := make([]string, 0, len(indexLengths)*variableTokenCount)
	for i, length := range indexLengths {
		if i < 6 {
			continue
		}
		for token := 0; token < variableTokenCount; token++ {
			indexOperands = append(indexOperands, fmt.Sprintf("i%d%s", length, TokenFromIndex(token)))
		}
	}

	echoOperands := make([]string, 0, len(indexLengths))
	for _, length := range indexLengths {
		if length > 9 {
			break
		}
		echoOperands = append(echoOperands, "e"+strconv.Itoa(length))
	}

	allOperands := append(singleOperands, directionalOperands...)
	allOperands = append(allOperands, magnitudeOperands...)
	allOperands = append(allOperands, echoOperands...)
	allOperands = append(allOperands, mirrorOperands...)
	allOperands = append(allOperands, mathOperands...)
	allOperands = append(allOperands, indexOperands...)

	filtered := allOperands[:0]
	for _, operand := range allOperands {
		if rollout == nil || rollout.OperationAllowed(string(operand[0])) {
			filtered = append(filtered, operand)
		}
	}
	return filtered
}

func IndexLengthCandidatesForInstruction(instructionLength int, varMax int) []int {
	maxLength := MaxIndexLengthForInstruction(instructionLength)
	smallLimit := MinInt(varMax, maxLength)
	lengths := make([]int, 0, smallLimit+1)
	for length := 1; length <= smallLimit; length++ {
		lengths = append(lengths, length)
	}
	if maxLength > smallLimit {
		lengths = append(lengths, maxLength)
	}
	return lengths
}

func (c *ICommands) ResolveInstructions(inst string) string {
	i := NewInstructions()
	current := inst
	for iter := 0; iter < variableTokenCount; iter++ {
		if !ContainsAlphabetic(current) {
			return current
		}
		index := []IndexEntry{}
		next := i.Interpret(current, 0, &index)
		if next == "" || next == "+" {
			return next
		}
		next = i.Devariablize(next, variableTokenCount)
		next = i.Render(next, 0, &index)
		if next == current {
			return next
		}
		current = next
	}
	return current
}

func (c *ICommands) Validate(modified, original string) bool {
	i := NewInstructions()

	modified, err := c.ReturnRenderedStrict(modified)
	if err != nil {
		return false
	}
	original, err = c.ReturnRenderedStrict(original)
	if err != nil {
		return false
	}

	if len(modified)%3 != 0 {
		return false
	}
	if len(modified) != len(original) {
		return false
	}

	return i.VerifyContinuity(modified, original)
}

func ValidateStrings(original, candidate string, clean bool) bool {
	commands := NewICommands()

	origRendered := commands.ReturnRendered(original)

	candidateInterpreted := commands.ReturnInterpret(candidate, 0, &[]IndexEntry{})
	candidateRendered := commands.ReturnRendered(candidate)

	valid := commands.Validate(candidate, original)

	if !clean {
		fmt.Println("[VALIDATE]")
		fmt.Printf("Original           : %s\n", original)
		fmt.Printf("Original rendered   : %s\n", origRendered)
		fmt.Printf("Candidate          : %s\n", candidate)
		fmt.Printf("Candidate interpreted: %s\n", candidateInterpreted)
		fmt.Printf("Candidate rendered   : %s\n", candidateRendered)
		fmt.Printf("Valid               : %v\n", valid)
	}

	return valid
}

func (c *ICommands) ReturnInterpret(instructions string, iteration int, index *[]IndexEntry) string {
	i := NewInstructions()

	interpreted := i.Interpret(instructions, iteration, &[]IndexEntry{})

	return interpreted
}

func (c *ICommands) ReturnRendered(instructions string) string {
	i := NewInstructions()
	index := make([]IndexEntry, 0)

	interpreted := i.Interpret(instructions, 0, &index)
	if interpreted == "" || interpreted == "+" {
		return "+"
	}

	devariablized := i.Devariablize(interpreted, variableTokenCount)

	rendered := i.Render(devariablized, 0, &index)

	resolved := c.ResolveInstructions(rendered)

	return resolved
}

func (c *ICommands) ReturnRenderedStrict(instructions string) (string, error) {
	rendered := c.ReturnRendered(instructions)
	if rendered == "" || rendered == "+" {
		return "", fmt.Errorf("failed to render instructions")
	}
	if !ContainsOnlyGlyphs(rendered) {
		return "", fmt.Errorf("rendered instructions still contain operands near %s",
			FirstOperandContext(rendered))
	}
	if len(rendered)%3 != 0 {
		return "", fmt.Errorf("rendered glyph length %d is not divisible by 3", len(rendered))
	}
	return rendered, nil
}
