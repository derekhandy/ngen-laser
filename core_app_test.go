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

func TestResolveDataRootFromWorkingDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	paths, err := ResolveDataRoot("")
	if err != nil {
		t.Fatalf("ResolveDataRoot returned an unexpected error: %v", err)
	}

	want := filepath.Join(cwd, "data")
	if paths.Root != want {
		t.Fatalf("data root = %q, want %q", paths.Root, want)
	}
}

func TestResolveDataRootOverride(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	paths, err := ResolveDataRoot(root)
	if err != nil {
		t.Fatalf("ResolveDataRoot returned an unexpected error: %v", err)
	}
	if paths.Root != root {
		t.Fatalf("data root = %q, want %q", paths.Root, root)
	}
	if paths.Config("training-config.json") != filepath.Join(root, "config", "training-config.json") {
		t.Fatalf("unexpected config path: %s", paths.Config("training-config.json"))
	}
}

func TestParseDataDirFlag(t *testing.T) {
	dataDir, rest, err := parseDataDirFlag([]string{"laser", "--data-dir", "/tmp/data", "train", "tools"})
	if err != nil {
		t.Fatal(err)
	}
	if dataDir != "/tmp/data" {
		t.Fatalf("data dir = %q, want /tmp/data", dataDir)
	}
	if len(rest) != 3 || rest[1] != "train" {
		t.Fatalf("unexpected remaining args: %v", rest)
	}
}
