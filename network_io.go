package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type SerializableNetwork struct {
	NetworkName string        `json:"networkName"`
	InputSize   int           `json:"inputSize"`
	Backbone    []SerialLayer `json:"backbone"`
	Heads       SerialHeads   `json:"heads"`
	Memory      *SerialMemory `json:"memory,omitempty"`
}

type SerialLayer struct {
	Weights    [][]float32 `json:"weights"`
	Biases     []float32   `json:"biases"`
	Activation string      `json:"activation"`
}

type SerialHeads struct {
	Index            SerialLayer `json:"index"`
	Operand          SerialLayer `json:"operand"`
	TranslationDir   SerialLayer `json:"translationDir"`
	TranslationMag   SerialLayer `json:"translationMag"`
	RotationAxis     SerialLayer `json:"rotationAxis"`
	ScalingMagnitude SerialLayer `json:"scalingMagnitude"`
	InsertionLength  SerialLayer `json:"insertionLength"`
	Stop             SerialLayer `json:"stop"`
}

type SerialMemory struct {
	RecurrentConfig RecurrentStateConfig   `json:"recurrentConfig"`
	AttentionConfig AttentionConfig        `json:"attentionConfig"`
	LayerCount      int                    `json:"layerCount"`
	RecurrentState  RecurrentState         `json:"RecurrentState"`
	GRUWeights      *SerialGRUWeights      `json:"gruWeights,omitempty"`
	Encoder         []SerialAttentionLayer `json:"encoder"`
}

type SerialGRUWeights struct {
	Wz []float32 `json:"wz"`
	Uz []float32 `json:"uz"`
	Bz []float32 `json:"bz"`
	Wr []float32 `json:"wr"`
	Ur []float32 `json:"ur"`
	Br []float32 `json:"br"`
	Wh []float32 `json:"wh"`
	Uh []float32 `json:"uh"`
	Bh []float32 `json:"bh"`
}

type SerialAttentionLayer struct {
	Config AttentionConfig `json:"config"`
	QProj  SerialLayer     `json:"qProj"`
	KProj  SerialLayer     `json:"kProj"`
	VProj  SerialLayer     `json:"vProj"`
	OProj  SerialLayer     `json:"oProj"`
	FFN1   SerialLayer     `json:"ffn1"`
	FFN2   SerialLayer     `json:"ffn2"`
}

func SaveNetwork(path string, net *Network) error {
	if net == nil {
		return fmt.Errorf("[!] Cannot save nil network")
	}
	EnsureNetworkMemory(net)

	payload := SerializableNetwork{
		NetworkName: net.networkName,
		InputSize:   net.inputSize,
		Backbone:    make([]SerialLayer, len(net.backbone)),
		Heads: SerialHeads{
			Index:            SerializeLayer(net.heads.Index),
			Operand:          SerializeLayer(net.heads.Operand),
			TranslationDir:   SerializeLayer(net.heads.TranslationDir),
			TranslationMag:   SerializeLayer(net.heads.TranslationMag),
			RotationAxis:     SerializeLayer(net.heads.RotationAxis),
			ScalingMagnitude: SerializeLayer(net.heads.ScalingMagnitude),
			InsertionLength:  SerializeLayer(net.heads.InsertionLength),
			Stop:             SerializeLayer(net.heads.Stop),
		},
		Memory: SerializeMemory(net.memory),
	}

	for i, layer := range net.backbone {
		payload.Backbone[i] = SerializeLayer(layer)
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return WriteFileAtomic(path, data, 0644)
}

func LoadNetwork(path string) (Network, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Network{}, err
	}

	var payload SerializableNetwork
	if err := json.Unmarshal(data, &payload); err != nil {
		return Network{}, err
	}
	if err := ValidateSerializableNetwork(payload); err != nil {
		return Network{}, fmt.Errorf("invalid network %q: %w", path, err)
	}

	net := Network{
		networkName: payload.NetworkName,
		inputSize:   payload.InputSize,
		backbone:    make([]Layer, len(payload.Backbone)),
	}

	for i, layer := range payload.Backbone {
		net.backbone[i] = DeserializeLayer(layer)
	}

	net.heads = OutputHeads{
		Index:            DeserializeLayer(payload.Heads.Index),
		Operand:          DeserializeLayer(payload.Heads.Operand),
		TranslationDir:   DeserializeLayer(payload.Heads.TranslationDir),
		TranslationMag:   DeserializeLayer(payload.Heads.TranslationMag),
		RotationAxis:     DeserializeLayer(payload.Heads.RotationAxis),
		ScalingMagnitude: DeserializeLayer(payload.Heads.ScalingMagnitude),
		InsertionLength:  DeserializeLayer(payload.Heads.InsertionLength),
		Stop:             DeserializeLayer(payload.Heads.Stop),
	}
	net.memory = DeserializeMemory(payload.Memory)
	EnsureNetworkMemory(&net)

	return net, nil
}

func ValidateSerializableNetwork(payload SerializableNetwork) error {
	if payload.InputSize <= 0 {
		return fmt.Errorf("input size must be positive, got %d", payload.InputSize)
	}
	if len(payload.Backbone) == 0 {
		return fmt.Errorf("backbone must contain at least one layer")
	}

	inputSize := payload.InputSize
	for i, layer := range payload.Backbone {
		outputSize, err := validateSerialLayer(layer, fmt.Sprintf("backbone[%d]", i), inputSize, 0)
		if err != nil {
			return err
		}
		inputSize = outputSize
	}

	heads := []struct {
		name  string
		layer SerialLayer
		rows  int
	}{
		{"heads.index", payload.Heads.Index, 1},
		{"heads.operand", payload.Heads.Operand, len(operandClasses)},
		{"heads.translationDir", payload.Heads.TranslationDir, len(directionClasses)},
		{"heads.translationMag", payload.Heads.TranslationMag, 1},
		{"heads.rotationAxis", payload.Heads.RotationAxis, len(axisClasses)},
		{"heads.scalingMagnitude", payload.Heads.ScalingMagnitude, 1},
		{"heads.insertionLength", payload.Heads.InsertionLength, 1},
		{"heads.stop", payload.Heads.Stop, 1},
	}
	for _, head := range heads {
		if _, err := validateSerialLayer(head.layer, head.name, inputSize, head.rows); err != nil {
			return err
		}
	}

	if payload.Memory == nil {
		return nil
	}
	memory := payload.Memory
	if memory.RecurrentConfig.HiddenSize != inputSize {
		return fmt.Errorf("memory recurrent size %d does not match network width %d", memory.RecurrentConfig.HiddenSize, inputSize)
	}
	if memory.AttentionConfig.HiddenSize != inputSize {
		return fmt.Errorf("memory attention size %d does not match network width %d", memory.AttentionConfig.HiddenSize, inputSize)
	}
	if memory.RecurrentConfig.UseGRU {
		if err := validateSerialGRUWeights(memory.GRUWeights, memory.RecurrentConfig.HiddenSize); err != nil {
			return err
		}
	} else if memory.GRUWeights != nil {
		if err := validateSerialGRUWeights(memory.GRUWeights, memory.RecurrentConfig.HiddenSize); err != nil {
			return err
		}
	}
	if memory.LayerCount <= 0 || memory.LayerCount != len(memory.Encoder) {
		return fmt.Errorf("memory layer count %d does not match encoder count %d", memory.LayerCount, len(memory.Encoder))
	}
	for i, layer := range memory.Encoder {
		prefix := fmt.Sprintf("memory.encoder[%d]", i)
		if _, err := validateSerialLayer(layer.QProj, prefix+".qProj", memory.RecurrentConfig.HiddenSize, memory.AttentionConfig.HiddenSize); err != nil {
			return err
		}
		if _, err := validateSerialLayer(layer.KProj, prefix+".kProj", memory.RecurrentConfig.HiddenSize, memory.AttentionConfig.HiddenSize); err != nil {
			return err
		}
		if _, err := validateSerialLayer(layer.VProj, prefix+".vProj", memory.RecurrentConfig.HiddenSize, memory.AttentionConfig.HiddenSize); err != nil {
			return err
		}
		for name, projection := range map[string]SerialLayer{
			"oProj": layer.OProj,
			"ffn1":  layer.FFN1,
			"ffn2":  layer.FFN2,
		} {
			if _, err := validateSerialLayer(projection, prefix+"."+name, memory.AttentionConfig.HiddenSize, memory.AttentionConfig.HiddenSize); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateSerialLayer(layer SerialLayer, name string, inputSize int, expectedRows int) (int, error) {
	if inputSize <= 0 {
		return 0, fmt.Errorf("%s has invalid input size %d", name, inputSize)
	}
	if len(layer.Weights) == 0 {
		return 0, fmt.Errorf("%s has no weight rows", name)
	}
	if expectedRows > 0 && len(layer.Weights) != expectedRows {
		return 0, fmt.Errorf("%s has %d weight rows, want %d", name, len(layer.Weights), expectedRows)
	}
	if len(layer.Biases) != len(layer.Weights) {
		return 0, fmt.Errorf("%s has %d biases for %d weight rows", name, len(layer.Biases), len(layer.Weights))
	}
	for row, weights := range layer.Weights {
		if len(weights) != inputSize {
			return 0, fmt.Errorf("%s weight row %d has width %d, want %d", name, row, len(weights), inputSize)
		}
	}
	return len(layer.Weights), nil
}

func validateSerialGRUWeights(weights *SerialGRUWeights, dim int) error {
	if weights == nil {
		return fmt.Errorf("GRU weights are missing")
	}
	matrixSize := dim * dim
	matrices := map[string][]float32{
		"wz": weights.Wz,
		"uz": weights.Uz,
		"wr": weights.Wr,
		"ur": weights.Ur,
		"wh": weights.Wh,
		"uh": weights.Uh,
	}
	for name, matrix := range matrices {
		if len(matrix) != matrixSize {
			return fmt.Errorf("GRU %s has %d values, want %d", name, len(matrix), matrixSize)
		}
	}
	biases := map[string][]float32{
		"bz": weights.Bz,
		"br": weights.Br,
		"bh": weights.Bh,
	}
	for name, bias := range biases {
		if len(bias) != dim {
			return fmt.Errorf("GRU %s has %d values, want %d", name, len(bias), dim)
		}
	}
	return nil
}

func SerializeLayer(layer Layer) SerialLayer {
	return SerialLayer{
		Weights:    layer.weights,
		Biases:     layer.biases,
		Activation: layer.activation,
	}
}

func DeserializeLayer(sl SerialLayer) Layer {
	return Layer{
		weights:    sl.Weights,
		biases:     sl.Biases,
		activation: sl.Activation,
	}
}

func SerializeMemory(memory *MemoryController) *SerialMemory {
	if memory == nil {
		return nil
	}
	layerCount := memory.layerCount
	if layerCount <= 0 {
		layerCount = len(memory.encoder)
	}

	vec := make([]float32, len(memory.state.vector))
	copy(vec, memory.state.vector)

	payload := &SerialMemory{
		RecurrentConfig: memory.recurrentConfig,
		AttentionConfig: memory.attentionConfig,
		LayerCount:      layerCount,
		GRUWeights:      SerializeGRUWeights(memory.weights),
		Encoder:         make([]SerialAttentionLayer, len(memory.encoder)),
	}
	for i, layer := range memory.encoder {
		payload.Encoder[i] = SerializeAttentionLayer(layer)
	}
	return payload
}

func DeserializeMemory(payload *SerialMemory) *MemoryController {
	if payload == nil {
		return nil
	}
	layerCount := payload.LayerCount
	if layerCount <= 0 {
		layerCount = len(payload.Encoder)
	}
	cfg := &MemoryConfig{
		rCfg:       payload.RecurrentConfig,
		aCfg:       payload.AttentionConfig,
		layerCount: layerCount,
	}

	memory := NewMemoryController(cfg)
	if payload.GRUWeights != nil {
		memory.weights = DeserializeGRUWeights(payload.GRUWeights)
	}

	if len(payload.Encoder) > 0 {
		memory.encoder = make([]AttentionLayer, len(payload.Encoder))
		for i, layer := range payload.Encoder {
			memory.encoder[i] = DeserializeAttentionLayer(layer)
		}
		memory.layerCount = len(memory.encoder)
	}

	if memory.state == nil || memory.state.vector == nil {
		if payload.RecurrentConfig.HiddenSize > 0 {
			memory.state = &RecurrentState{
				config: payload.RecurrentConfig,
				vector: make([]float32, payload.RecurrentConfig.HiddenSize),
			}
		}
	}

	return memory
}

func SerializeGRUWeights(weights *GRUWeights) *SerialGRUWeights {
	if weights == nil {
		return nil
	}
	return &SerialGRUWeights{
		Wz: weights.Wz, Uz: weights.Uz, Bz: weights.Bz,
		Wr: weights.Wr, Ur: weights.Ur, Br: weights.Br,
		Wh: weights.Wh, Uh: weights.Uh, Bh: weights.Bh,
	}
}

func DeserializeGRUWeights(weights *SerialGRUWeights) *GRUWeights {
	if weights == nil {
		return nil
	}
	return &GRUWeights{
		Wz: weights.Wz, Uz: weights.Uz, Bz: weights.Bz,
		Wr: weights.Wr, Ur: weights.Ur, Br: weights.Br,
		Wh: weights.Wh, Uh: weights.Uh, Bh: weights.Bh,
	}
}

func SerializeAttentionLayer(layer AttentionLayer) SerialAttentionLayer {
	return SerialAttentionLayer{
		Config: layer.config,
		QProj:  SerializeLayer(layer.qProj),
		KProj:  SerializeLayer(layer.kProj),
		VProj:  SerializeLayer(layer.vProj),
		OProj:  SerializeLayer(layer.oProj),
		FFN1:   SerializeLayer(layer.ffn1),
		FFN2:   SerializeLayer(layer.ffn2),
	}
}

func DeserializeAttentionLayer(payload SerialAttentionLayer) AttentionLayer {
	layer := AttentionLayer{
		config: payload.Config,
		qProj:  DeserializeLayer(payload.QProj),
		kProj:  DeserializeLayer(payload.KProj),
		vProj:  DeserializeLayer(payload.VProj),
		oProj:  DeserializeLayer(payload.OProj),
		ffn1:   DeserializeLayer(payload.FFN1),
		ffn2:   DeserializeLayer(payload.FFN2),
	}

	layer.config = EnsureHeads(layer.config)

	hSize := payload.Config.HiddenSize
	if hSize > 0 {
		AllocateAttentionBuffers(&layer)
	}

	return layer
}

func InitializePopulation(inputSize int, hiddenLayers []int, populationSize int) []*Network {
	population := make([]*Network, populationSize)
	for i := 0; i < populationSize; i++ {
		population[i] = InitializeNetwork(inputSize, hiddenLayers, TotalOutputSize(), i)
	}
	return population
}

func SeedEnvironmentWithBestChain(env *CompressionEnvironment) {
	chain := env.BestChain()
	if len(chain) == 0 {
		return
	}

	DebugPrint(env.rollout, fmt.Sprintf("BEST CHAIN\n%v", chain), false)

	env.Reset()
}

func InjectPreviousBest(size int, population []*Network, best *Network, cfg ExhaustiveConfig) {
	if best == nil || len(best.backbone) == 0 {
		return
	}

	slots := maxSavedElites

	if slots > size {
		slots = size
	}

	for i := 0; i < slots; i++ {
		population[i] = CloneNetwork(best)
	}
}
