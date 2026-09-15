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
