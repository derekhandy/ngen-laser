package main

import (
	"os"
	"testing"
)

func BenchmarkDeflateBitOps(b *testing.B) {
	sample, err := os.ReadFile("C:/Users/Doug/Misc/Projects/training/samples/ct - network-record.lzr")
	if err != nil {
		return
	}

	input := InstructionsToBytesRuntime(string(sample))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = zstdEncoder.EncodeAll(input, nil)
	}
}
