package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrainingFlagsLoadRecordThresholdSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "training-flags.json")
	data := []byte(`{"thresholdCompToWrite":0.7,"endTrainingOnThreshold":true}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	flags, err := LoadTrainingFlags(path)
	if err != nil {
		t.Fatalf("LoadTrainingFlags returned an unexpected error: %v", err)
	}
	if flags.ThresholdCompToWrite != 0.7 {
		t.Fatalf("thresholdCompToWrite = %v, want 0.7", flags.ThresholdCompToWrite)
	}
	if !flags.EndTrainingOnThreshold {
		t.Fatal("endTrainingOnThreshold was not loaded")
	}
}

func TestDefaultTrainingFlagsDoNotStopAtThreshold(t *testing.T) {
	flags := LoadDefaultTrainingFlags()
	if flags.ThresholdCompToWrite != 0 || flags.EndTrainingOnThreshold {
		t.Fatalf("unexpected default record flags: %+v", flags)
	}
}
