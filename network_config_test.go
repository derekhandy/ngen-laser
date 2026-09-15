package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareTreeIncludesAllInstructionRoots(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	if err := os.MkdirAll(first, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(first, "one.txt"),
		filepath.Join(second, "two.txt"),
	} {
		if err := os.WriteFile(path, []byte(";;;;"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	tree, err := PrepareTree([]string{first, second})
	if err != nil {
		t.Fatalf("PrepareTree returned an unexpected error: %v", err)
	}
	defer func() {
		for _, tempPath := range tree.TempPaths {
			_ = os.RemoveAll(tempPath)
		}
	}()

	if len(tree.Containers) != 2 {
		t.Fatalf("got %d containers, want 2", len(tree.Containers))
	}
	seen := make(map[string]bool)
	for _, container := range tree.Containers {
		seen[container.RelativePath] = true
	}
	if !seen["0-first/one.txt"] || !seen["1-second/two.txt"] {
		t.Fatalf("merged tree paths = %v, want both namespaced roots", seen)
	}
}

func TestPrepareTreePreservesSingleRootPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "one.txt")
	if err := os.WriteFile(path, []byte(";;;;"), 0644); err != nil {
		t.Fatal(err)
	}

	tree, err := PrepareTree([]string{root})
	if err != nil {
		t.Fatalf("PrepareTree returned an unexpected error: %v", err)
	}
	defer func() {
		for _, tempPath := range tree.TempPaths {
			_ = os.RemoveAll(tempPath)
		}
	}()
	if len(tree.Containers) != 1 || tree.Containers[0].RelativePath != "one.txt" {
		t.Fatalf("single-root path = %v, want one.txt", tree.Containers)
	}
}

func TestPrepareTreeAcceptsFileInstructionRoots(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "one.txt")
	if err := os.WriteFile(file, []byte(";;;;"), 0644); err != nil {
		t.Fatal(err)
	}

	tree, err := PrepareTree([]string{file})
	if err != nil {
		t.Fatalf("PrepareTree returned an unexpected error: %v", err)
	}
	defer func() {
		for _, tempPath := range tree.TempPaths {
			_ = os.RemoveAll(tempPath)
		}
	}()
	if len(tree.Containers) != 1 || tree.Containers[0].RelativePath != "one.txt" {
		t.Fatalf("file-root path = %v, want one.txt", tree.Containers)
	}
}
