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
