package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageRecordFramingEscapesDelimiterBytes(t *testing.T) {
	payload := []byte{'p', '/', '~', 'a', 0xff}
	record := encodePackageRecord("part0.isp", "nested/~file.txt", payload)

	records, err := parsePackageRecords(record)
	if err != nil {
		t.Fatalf("safe package record failed to parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("parsed %d records, want 1", len(records))
	}
	if records[0].archivedPath != "nested/~file.txt" {
		t.Fatalf("archived path = %q, want nested/~file.txt", records[0].archivedPath)
	}
	if !bytes.Equal([]byte(records[0].payload), payload) {
		t.Fatalf("payload changed during framing: %v", []byte(records[0].payload))
	}

	root := t.TempDir()
	encoded := InstructionsToBytesRuntime(string(TextToInstructions([]byte("payload"))))
	safePackage := string(append(append([]byte{}, packageFormat...), encodePackageRecord("part0.isp", "nested/~file.txt", encoded)...))
	if err := RestoreDirectoryTo("test.lzr", safePackage, "", root); err != nil {
		t.Fatalf("safe package failed to restore: %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(root, "nested", "~file.txt"))
	if err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
	if string(restored) != "payload" {
		t.Fatalf("restored payload = %q, want payload", restored)
	}
}

func TestRestorePathWithinRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "restore")

	got, err := restorePathWithinRoot(root, "nested/file.txt")
	if err != nil {
		t.Fatalf("valid relative path rejected: %v", err)
	}
	want := filepath.Join(root, "nested", "file.txt")
	if got != want {
		t.Fatalf("restore path = %q, want %q", got, want)
	}

	for _, archivedPath := range []string{
		"..\\outside.txt",
		"nested/../../outside.txt",
		filepath.Join(string(filepath.Separator), "outside.txt"),
		"C:\\outside.txt",
		"\\\\server\\share\\outside.txt",
	} {
		if _, err := restorePathWithinRoot(root, archivedPath); err == nil {
			t.Errorf("malicious path %q was accepted", archivedPath)
		}
	}
}

func TestGetContextPathValidatesRootRelationship(t *testing.T) {
	root := t.TempDir()
	got, err := GetContextPath(filepath.Join(root, "nested", "file.txt"), root)
	if err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
	if got != "nested/file.txt" {
		t.Fatalf("context path = %q, want nested/file.txt", got)
	}

	for _, test := range []struct {
		name string
		path string
		root string
	}{
		{name: "empty path", path: "", root: root},
		{name: "empty root", path: filepath.Join(root, "file.txt"), root: ""},
		{name: "root itself", path: root, root: root},
		{name: "outside root", path: filepath.Join(filepath.Dir(root), "file.txt"), root: root},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := GetContextPath(test.path, test.root); err == nil {
				t.Fatalf("accepted invalid path/root pair: %q, %q", test.path, test.root)
			}
		})
	}
}

func TestRestoreDirectoryRejectsPathBeforeWriting(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	part := string(InstructionsToBytesRuntime(TextToInstructions([]byte("payload"))))
	packageData := string(append(append([]byte{}, packageFormat...), encodePackageRecord("part0.isp", "../outside.txt", []byte(part))...))

	err := RestoreDirectoryTo("test.lzr", packageData, "", root)
	if err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
		t.Fatalf("outside file was created: %v", statErr)
	}
}

func TestRestoreDirectoryRejectsCorruptPayloadBeforeWriting(t *testing.T) {
	root := t.TempDir()
	packageData := string(append(append([]byte{}, packageFormat...), encodePackageRecord("part0.isp", "file.txt", []byte("not-a-valid-encoded-payload"))...))

	err := RestoreDirectoryTo("test.lzr", packageData, "", root)
	if err == nil {
		t.Fatal("expected corrupt payload to be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(root, "file.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("file was created from corrupt payload: %v", statErr)
	}
}

func TestRestoreDirectoryRejectsMalformedPartRecords(t *testing.T) {
	part := string(InstructionsToBytesRuntime(TextToInstructions([]byte("payload"))))
	tests := []struct {
		name string
		data string
	}{
		{
			name: "trailing fragment",
			data: string(append(append(append([]byte{}, packageFormat...), encodePackageRecord("part0.isp", "file.txt", []byte(part))...), []byte("trailing")...)),
		},
		{
			name: "duplicate index",
			data: string(append(append(append([]byte{}, packageFormat...), encodePackageRecord("part0.isp", "file.txt", []byte(part))...), encodePackageRecord("part0.isp", "file.txt", []byte(part))...)),
		},
		{
			name: "missing index",
			data: string(append(append(append([]byte{}, packageFormat...), encodePackageRecord("part0.isp", "file.txt", []byte(part))...), encodePackageRecord("part2.isp", "file.txt", []byte(part))...)),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := RestoreDirectoryTo("test.lzr", test.data, "", root); err == nil {
				t.Fatal("expected malformed package to be rejected")
			}
			if _, err := os.Stat(filepath.Join(root, "file.txt")); !os.IsNotExist(err) {
				t.Fatalf("file was written from malformed package: %v", err)
			}
		})
	}
}

func TestPackagePartCountUsesCeilingAndPreservesEmptyFiles(t *testing.T) {
	for _, test := range []struct {
		name   string
		length int
		want   int
	}{
		{name: "empty", length: 0, want: 1},
		{name: "exact packet", length: packetSize, want: 1},
		{name: "one byte over", length: packetSize + 1, want: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := packagePartCount(test.length, packetSize); got != test.want {
				t.Fatalf("packagePartCount(%d) = %d, want %d", test.length, got, test.want)
			}
		})
	}

	restored, err := RestoreFileRuntime(string(InstructionsToBytesRuntime("")))
	if err != nil {
		t.Fatalf("empty payload failed to restore: %v", err)
	}
	if len(restored) != 0 {
		t.Fatalf("empty payload restored %d bytes", len(restored))
	}
}

func TestWriteNetworkOutputSealsAllParts(t *testing.T) {
	dir := t.TempDir()
	part0 := filepath.Join(dir, "part0.isp~")
	part1 := filepath.Join(dir, "part1.isp~")
	for _, path := range []string{part0, part1} {
		if err := os.WriteFile(path, []byte("source"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	container := Container{Remaining: 2}
	if err := WriteNetworkOutput(&container, "compressed-0", part0); err != nil {
		t.Fatal(err)
	}
	if container.Remaining != 1 {
		t.Fatalf("remaining after first part = %d, want 1", container.Remaining)
	}
	if _, err := os.Stat(part1); err != nil {
		t.Fatalf("second source part was removed too early: %v", err)
	}

	if err := WriteNetworkOutput(&container, "compressed-1", part1); err != nil {
		t.Fatal(err)
	}
	if container.Remaining != 0 {
		t.Fatalf("remaining after final part = %d, want 0", container.Remaining)
	}
	for _, path := range []string{part0, part1} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("source part %s still exists: %v", path, err)
		}
	}
}

func TestWriteNetworkOutputRejectsInvalidPartPath(t *testing.T) {
	container := &Container{Remaining: 1}
	for _, path := range []string{"", "part0.isp", "part0.isp~~"} {
		if err := WriteNetworkOutput(container, "compressed", path); err == nil {
			t.Errorf("WriteNetworkOutput accepted invalid path %q", path)
		}
	}
}

func TestMakePackageKeepsPartsWhenPackageWriteFails(t *testing.T) {
	tmpDir := t.TempDir()
	containerDir := filepath.Join(tmpDir, "container")
	if err := os.Mkdir(containerDir, 0755); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(containerDir, "part0.isp")
	if err := os.WriteFile(part, []byte("compressed"), 0644); err != nil {
		t.Fatal(err)
	}

	tree := &Tree{
		Name: "package",
		Path: filepath.Join(tmpDir, "missing-parent"),
		Containers: []Container{{
			TmpPath:      containerDir,
			RelativePath: "file.txt",
		}},
	}
	if err := MakePackage(tree); err == nil {
		t.Fatal("expected package write to fail")
	}
	if _, err := os.Stat(part); err != nil {
		t.Fatalf("temporary part was removed after package write failure: %v", err)
	}
}

func TestBuildPackageRemovesOnlyOwnedWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	containerDir := filepath.Join(workspace, "container")
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(containerDir, "part0.isp"), []byte("compressed"), 0644); err != nil {
		t.Fatal(err)
	}

	tree := &Tree{
		Name:     "package",
		Path:     root,
		TempPath: workspace,
		Containers: []Container{{
			TmpPath:      containerDir,
			RelativePath: "file.txt",
		}},
	}
	if err := BuildPackage(tree); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("owned workspace still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "package.lzr")); err != nil {
		t.Fatalf("package was not created: %v", err)
	}
}
