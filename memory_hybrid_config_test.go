package main

import "testing"

func TestDefaultMemoryConfigUsesCustomHiddenSizeForRecurrentState(t *testing.T) {
	for _, hiddenSize := range []int{64, 256} {
		config := DefaultMemoryConfig(hiddenSize)
		if config.rCfg.HiddenSize != hiddenSize {
			t.Fatalf("hidden size %d: recurrent size = %d", hiddenSize, config.rCfg.HiddenSize)
		}
		if config.aCfg.HiddenSize != hiddenSize {
			t.Fatalf("hidden size %d: attention size = %d", hiddenSize, config.aCfg.HiddenSize)
		}
	}
}
