package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
)

func WriteNetworks(rollout *Rollout, network []*Network) {
	if err := CheckWritingParameters(rollout, len(network), maxSavedElites); err != nil {
		fmt.Printf("[WARN] Error saving networks: %v", err)
	}

	fmt.Printf("\nWriting elite networks to disk...\n")

	savedCount := 0
	for i := range MinInt(len(network), maxSavedElites) {
		filename := rollout.Paths.Weights(fmt.Sprintf("elite_%d.json", i))
		if err := SaveNetwork(filename, network[i]); err != nil {
			fmt.Printf("[WARN] failed to save %s: %v", filename, err)
			continue
		}

		savedCount++
	}
	if savedCount == 0 {
		fmt.Printf("[WARN] no elites were saved")
	}
}

func CheckWritingParameters(rollout *Rollout, size int, targetCount int) error {
	if size == 0 {
		return fmt.Errorf("[WARN] writeNetworks called with empty population")
	}

	if err := os.MkdirAll(rollout.Paths.WeightsDir(), 0755); err != nil {
		return fmt.Errorf("[WARN] could not create weights folder: %v", err)
	}

	if targetCount == 0 {
		return fmt.Errorf("[WARN] maxSavedElite is set to 0")
	}

	return nil
}

func EvolvePopulation(network []*Network, fitnesses []float32, cfg ExhaustiveConfig) ([]*Network, *Network, float32, error) {
	size := MinInt(len(network), len(fitnesses))
	if size == 0 {
		return nil, nil, 0, fmt.Errorf("[!] Size == 0")
	}
	if err := ValidatePopulationPercentages(cfg); err != nil {
		return nil, nil, 0, err
	}

	eliteCount := PercentCount(cfg.ElitePercent, size, 1)
	mutatedEliteCount := PercentCount(cfg.MutatedElitePercent, size, 0)
	crossoverCount := PercentCount(cfg.CrossoverPercent, size, 0)
	freshCount := PercentCount(cfg.NewInitPercent, size, 0)

	remaining := size - eliteCount
	if mutatedEliteCount > remaining {
		mutatedEliteCount = remaining
	}
	remaining -= mutatedEliteCount
	if freshCount > remaining {
		freshCount = remaining
	}

	indices := RankedNetworkIndices(fitnesses, size)
	var template *Network
	for _, idx := range indices {
		if network[idx] != nil {
			template = network[idx]
			break
		}
	}
	if template == nil {
		template = InitializeNetwork(defaultInputSize, defaultHiddenLayers, TotalOutputSize(), 0)
	}
	bestNetwork := template
	ranked := make([]int, len(indices))
	copy(ranked, indices)

	newNetworks := make([]*Network, size)
	cursor := 0

	for i := 0; i < eliteCount && cursor < size; i++ {
		parent := network[ranked[i%len(ranked)]]
		if parent == nil {
			parent = template
		}
		newNetworks[cursor] = CloneNetwork(parent)
		cursor++
	}

	for i := 0; i < mutatedEliteCount && cursor < size; i++ {
		parent := network[ranked[i%len(ranked)]]
		if parent == nil {
			parent = template
		}
		child := CloneNetwork(parent)
		if child == nil {
			child = InitializeNetwork(defaultInputSize, defaultHiddenLayers, TotalOutputSize(), cursor)
		}
		newNetworks[cursor] = MutateNetwork(child, cfg.MutationPercent, cfg.MutationStrength)
		cursor++
	}

	for i := 0; i < freshCount && cursor < size; i++ {
		newNetworks[cursor] = InitializeNetwork(defaultInputSize, defaultHiddenLayers, TotalOutputSize(), cursor)
		cursor++
	}

	for i := 0; i < crossoverCount && cursor < size; i++ {
		parent1 := TournamentSelect(network[:size], fitnesses[:size])
		parent2 := TournamentSelect(network[:size], fitnesses[:size])
		if parent1 == nil {
			parent1 = template
		}
		if parent2 == nil {
			parent2 = template
		}

		child := Crossover(parent1, parent2)
		if child == nil {
			child = CloneNetwork(parent1)
		}
		if child == nil {
			child = InitializeNetwork(defaultInputSize, defaultHiddenLayers, TotalOutputSize(), cursor)
		}

		if rand.Float32() < crossoverMutateChance {
			child = MutateNetwork(child, cfg.MutationPercent, cfg.MutationStrength)
		}
		newNetworks[cursor] = child
		cursor++
	}

	for cursor < size {
		newNetworks[cursor] = CloneNetwork(template)
		if newNetworks[cursor] == nil {
			newNetworks[cursor] = InitializeNetwork(defaultInputSize, defaultHiddenLayers, TotalOutputSize(), cursor)
		}
		cursor++
	}

	bestFitness := float32(math.Inf(-1))
	for i := range len(fitnesses) {
		if fitnesses[i] > bestFitness {
			bestFitness = fitnesses[i]
		}
	}

	return newNetworks, bestNetwork, bestFitness, nil
}

func ValidatePopulationPercentages(cfg ExhaustiveConfig) error {
	percentages := []struct {
		name  string
		value float32
	}{
		{"elite", cfg.ElitePercent},
		{"mutated elite", cfg.MutatedElitePercent},
		{"crossover", cfg.CrossoverPercent},
		{"new network", cfg.NewInitPercent},
	}

	var total float32
	for _, percentage := range percentages {
		if math.IsNaN(float64(percentage.value)) || math.IsInf(float64(percentage.value), 0) || percentage.value < 0 || percentage.value > 1 {
			return fmt.Errorf("[!] %s population percentage must be between 0 and 1, got %v", percentage.name, percentage.value)
		}
		total += percentage.value
	}
	if math.Abs(float64(total-1)) > 1e-5 {
		return fmt.Errorf("[!] Population percentages must total 1, got %v", total)
	}
	return nil
}

func TournamentSelect(population []*Network, fitnesses []float32) *Network {
	size := len(population)
	if size == 0 {
		return nil
	}

	tournamentSize := 3
	if tournamentSize > size {
		tournamentSize = size
	}

	seen := make(map[int]bool, tournamentSize)
	bestIdx := rand.Intn(size)
	seen[bestIdx] = true

	for len(seen) < tournamentSize {
		idx := rand.Intn(size)
		if seen[idx] {
			continue
		}
		seen[idx] = true

		if NormalizedFitness(fitnesses[idx]) > NormalizedFitness(fitnesses[bestIdx]) {
			bestIdx = idx
		}
	}

	return population[bestIdx]
}

func Crossover(parent1, parent2 *Network) *Network {
	if parent1 == nil {
		return CloneNetwork(parent2)
	}
	if parent2 == nil {
		return CloneNetwork(parent1)
	}
	child := CloneNetwork(parent1)
	if child == nil {
		return nil
	}

	applyCrossover := func(dst *Layer, src *Layer, overwriteProb, blendProb float32) {
		if rand.Float32() < overwriteProb {
			OverwriteLayer(dst, *src)
		} else {
			BlendLayer(dst, src, blendProb)
		}
	}

	for i := range child.backbone {
		if i < len(parent2.backbone) {
			applyCrossover(&child.backbone[i], &parent2.backbone[i], 0.3, 0.5)
		}
	}

	applyCrossover(&child.heads.Index, &parent2.heads.Index, 0.0, 0.2)
	applyCrossover(&child.heads.Operand, &parent2.heads.Operand, 0.0, 0.2)
	applyCrossover(&child.heads.TranslationDir, &parent2.heads.TranslationDir, 0.0, 0.2)
	applyCrossover(&child.heads.TranslationMag, &parent2.heads.TranslationMag, 0.0, 0.2)
	applyCrossover(&child.heads.RotationAxis, &parent2.heads.RotationAxis, 0.0, 0.2)
	applyCrossover(&child.heads.ScalingMagnitude, &parent2.heads.ScalingMagnitude, 0.0, 0.2)
	applyCrossover(&child.heads.InsertionLength, &parent2.heads.InsertionLength, 0.0, 0.2)
	applyCrossover(&child.heads.Stop, &parent2.heads.Stop, 0.0, 0.2)

	EnsureNetworkMemory(child)
	return child
}

func CloneNetwork(net *Network) *Network {
	if net == nil {
		return nil
	}

	clone := &Network{
		networkName: net.networkName,
		inputSize:   net.inputSize,
	}

	clone.backbone = make([]Layer, len(net.backbone))
	for i := range net.backbone {
		clone.backbone[i] = net.backbone[i].DeepCopy()
	}

	clone.heads = net.heads.DeepCopy()

	clone.memory = CloneMemoryController(EnsureNetworkMemory(net))

	return clone
}

func (l Layer) DeepCopy() Layer {
	newBiases := make([]float32, len(l.biases))
	copy(newBiases, l.biases)

	newWeights := make([][]float32, len(l.weights))
	for i := range l.weights {
		newWeights[i] = make([]float32, len(l.weights[i]))
		copy(newWeights[i], l.weights[i])
	}

	return Layer{
		weights:    newWeights,
		biases:     newBiases,
		activation: l.activation,
	}
}

func (oh OutputHeads) DeepCopy() OutputHeads {
	return OutputHeads{
		Index:            oh.Index.DeepCopy(),
		Operand:          oh.Operand.DeepCopy(),
		TranslationDir:   oh.TranslationDir.DeepCopy(),
		TranslationMag:   oh.TranslationMag.DeepCopy(),
		RotationAxis:     oh.RotationAxis.DeepCopy(),
		ScalingMagnitude: oh.ScalingMagnitude.DeepCopy(),
		InsertionLength:  oh.InsertionLength.DeepCopy(),
		Stop:             oh.Stop.DeepCopy(),
	}
}

func MutateNetwork(network *Network, mutatePercent float32, mutationStrength float32) *Network {
	if network == nil {
		return nil
	}
	memory := EnsureNetworkMemory(network)
	mutatePercent = ClampMutationPercent(mutatePercent)
	if mutationStrength < 0 || math.IsNaN(float64(mutationStrength)) {
		mutationStrength = 0
	}

	for i := range network.backbone {
		MutateLayer(&network.backbone[i], mutatePercent, mutationStrength)
	}

	MutateLayer(&network.heads.Index, mutatePercent, mutationStrength)
	MutateLayer(&network.heads.Operand, mutatePercent, mutationStrength)
	MutateLayer(&network.heads.TranslationDir, mutatePercent, mutationStrength)
	MutateLayer(&network.heads.TranslationMag, mutatePercent, mutationStrength)
	MutateLayer(&network.heads.RotationAxis, mutatePercent, mutationStrength)
	MutateLayer(&network.heads.ScalingMagnitude, mutatePercent, mutationStrength)
	MutateLayer(&network.heads.InsertionLength, mutatePercent, mutationStrength)
	MutateLayer(&network.heads.Stop, mutatePercent, mutationStrength)

	if memory != nil {
		MutateMemory(memory, mutatePercent, mutationStrength)
	}

	return network
}

func CloneLayer(layer Layer) Layer {
	cloned := Layer{
		weights:    make([][]float32, len(layer.weights)),
		biases:     make([]float32, len(layer.biases)),
		activation: layer.activation,
	}
	for i := range layer.weights {
		cloned.weights[i] = make([]float32, len(layer.weights[i]))
		copy(cloned.weights[i], layer.weights[i])
	}
	copy(cloned.biases, layer.biases)
	return cloned
}

func MutateLayer(layer *Layer, mutatePercent float32, mutationStrength float32) {
	if layer == nil || mutatePercent <= 0 || mutationStrength == 0 {
		return
	}
	for i := range layer.weights {
		for j := range layer.weights[i] {
			if rand.Float32() < mutatePercent {
				delta := float32(rand.NormFloat64()) * mutationStrength
				layer.weights[i][j] += delta
				if layer.weights[i][j] > 5 {
					layer.weights[i][j] = 5
				} else if layer.weights[i][j] < -5 {
					layer.weights[i][j] = -5
				}
			}
		}
	}
	for i := range layer.biases {
		if rand.Float32() < mutatePercent {
			delta := float32(rand.NormFloat64()) * mutationStrength
			layer.biases[i] += delta
			if layer.biases[i] > 5 {
				layer.biases[i] = 5
			} else if layer.biases[i] < -5 {
				layer.biases[i] = -5
			}
		}
	}
}

func OverwriteLayer(dst *Layer, src Layer) {
	if len(dst.weights) != len(src.weights) {
		return
	}
	for i := range dst.weights {
		if len(dst.weights[i]) == len(src.weights[i]) {
			copy(dst.weights[i], src.weights[i])
		}
	}
	if len(dst.biases) == len(src.biases) {
		copy(dst.biases, src.biases)
	}
}

func BlendLayer(dst *Layer, src *Layer, swapProb float32) {
	if len(dst.weights) != len(src.weights) {
		return
	}
	for i := range dst.weights {
		if len(dst.weights[i]) != len(src.weights[i]) {
			continue
		}
		if rand.Float32() > swapProb {
			continue
		}
		copy(dst.weights[i], src.weights[i])
	}
	if len(dst.biases) == len(src.biases) {
		for i := range dst.biases {
			if rand.Float32() < swapProb {
				dst.biases[i] = src.biases[i]
			}
		}
	}
}

func RankedNetworkIndices(fitnesses []float32, size int) []int {
	size = MinInt(size, len(fitnesses))
	indices := make([]int, len(fitnesses))
	for i := range indices {
		indices[i] = i
	}

	norms := make([]float32, len(fitnesses))
	for i, f := range fitnesses {
		norms[i] = NormalizedFitness(f)
	}

	sort.SliceStable(indices, func(i, j int) bool {
		return norms[indices[i]] > norms[indices[j]]
	})

	return indices[:size]
}

func PercentCount(percent float32, size int, minCount int) int {
	if size <= 0 {
		return 0
	}
	if math.IsNaN(float64(percent)) || percent <= 0 {
		return MinInt(size, minCount)
	}
	if percent > 1 {
		percent = 1
	}
	count := int(math.Round(float64(percent * float32(size))))
	if count < minCount {
		count = minCount
	}
	if count > size {
		count = size
	}
	return count
}

func ClampMutationPercent(percent float32) float32 {
	if math.IsNaN(float64(percent)) || percent < 0 {
		return 0
	}
	if percent > 1 {
		return 1
	}
	return percent
}

func NormalizedFitness(fitness float32) float32 {
	if math.IsNaN(float64(fitness)) {
		return float32(math.Inf(-1))
	}
	return fitness
}
