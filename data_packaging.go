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
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const laserheader = "‰LASER"
const laserseperator = "+"
const laserfooter = "PACKAGE¸"

type packageRecord struct {
	name         string
	archivedPath string
	payload      string
}

type Tree struct {
	Name       string
	Path       string
	TempPath   string
	TempPaths  []string
	Containers []Container
}

type Container struct {
	Name         string
	Id           string
	TmpPath      string
	FullPath     string
	RelativePath string
	Lzrs         []string
	Remaining    int
}

func WriteNetworkOutput(container *Container, instructions string, path string) error {
	if container == nil {
		return fmt.Errorf("write network output %s: container is nil", path)
	}
	if !strings.HasSuffix(path, ".isp~") {
		return fmt.Errorf("write network output %s: expected .isp~ temporary part path", path)
	}

	outputPath := strings.TrimSuffix(path, "~")

	if _, err := NewICommands().ReturnRenderedStrict(instructions); err != nil {
		fmt.Fprintf(os.Stderr, "[write] .isp %s FAILS validation: %v\n", outputPath, err)
		return fmt.Errorf("write network output %s: %v", outputPath, err)
	}

	if err := os.WriteFile(outputPath, []byte(instructions), 0644); err != nil {
		return fmt.Errorf("write network output %s: %w", path, err)
	}

	container.Remaining--
	if container.Remaining != 0 {
		return nil
	}

	if err := SealContainer(filepath.Dir(path)); err != nil {
		return fmt.Errorf("seal container %s: %w", filepath.Dir(path), err)
	}

	return nil
}

func DirectoryToContainers(basePath string) (*Tree, error) {
	fileName := filepath.Base(basePath)
	fileLocation := filepath.Dir(basePath)
	contextRoot := basePath
	if info, err := os.Stat(basePath); err != nil {
		return nil, fmt.Errorf("stat instruction root %s: %w", basePath, err)
	} else if !info.IsDir() {
		contextRoot = fileLocation
	}

	tempPath, err := os.MkdirTemp("", "laser-")
	if err != nil {
		return nil, fmt.Errorf("create temporary workspace: %w", err)
	}
	cleanupTempPath := true
	defer func() {
		if cleanupTempPath {
			_ = os.RemoveAll(tempPath)
		}
	}()

	tree := Tree{}

	var containers []Container
	err = filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("[!] os.ReadFile error")
			}

			contextPath, err := GetContextPath(path, contextRoot)
			if err != nil {
				return fmt.Errorf("derive context path for %s: %w", path, err)
			}
			containerPath := GetContainerPath(fileName, contextPath)
			containerTempPath := filepath.Join(tempPath, containerPath)
			if err := os.Mkdir(containerTempPath, 0755); err != nil {
				return fmt.Errorf("create container directory: %w", err)
			}
			err = os.WriteFile(filepath.Join(containerTempPath, "contents"), data, 0644)
			if err != nil {
				return fmt.Errorf("[!] os.WriteFile error: %v", err)
			}
			fullPath := containerTempPath

			lzrs, err := SliceContents(containerTempPath, containerPath)
			if err != nil {
				return err
			}

			container := Container{
				Name:         filepath.Base(path),
				Id:           containerPath,
				TmpPath:      fullPath,
				FullPath:     path,
				RelativePath: filepath.ToSlash(contextPath),
				Lzrs:         lzrs,
				Remaining:    len(lzrs),
			}

			containers = append(containers, container)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	tree = Tree{
		Name:       fileName,
		Path:       fileLocation,
		TempPath:   tempPath,
		TempPaths:  []string{tempPath},
		Containers: containers,
	}
	cleanupTempPath = false

	return &tree, nil
}

func RestorePackage(path string) error {
	ClearLogs(nil)

	LogLoading(fmt.Sprintf("validating package '%s'", path))

	if filepath.Ext(path) != ".lzr" {
		PrintConfirm("fail")
		return fmt.Errorf("file type is invalid")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	str := string(data)

	header := MakeHeader(filepath.Base(path), 0)

	if !strings.Contains(str, header) {
		PrintConfirm("fail")
		return fmt.Errorf("file does not contain a valid header")
	}

	PrintConfirm("pass")

	LogLoading("Checking nominal conflicts")

	dir := filepath.Dir(path)
	fileName := filepath.Base(path)

	newPath := dir + "/" + fileName
	newPath = newPath[:len(newPath)-4]
	restorePath := newPath + "-restore"

	if PathExists(restorePath) {
		PrintConfirm("fail")
		return fmt.Errorf("file '%s' already exists at '%s'", filepath.Base(newPath), filepath.Dir(newPath))
	}

	PrintConfirm("pass")

	err = RestorePackageTo(path, restorePath)
	if err != nil {
		return fmt.Errorf("error rebuilding directory, %v", err)
	}

	fmt.Printf(Green + "\nfile successfully unpacked" + White)
	return nil
}

func RestorePackageTo(path string, restoreRoot string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	str := string(data)
	header := MakeHeader(filepath.Base(path), 0)
	if !strings.HasPrefix(str, header) {
		return fmt.Errorf("file does not contain a valid header")
	}

	return RestoreDirectoryTo(path, str, header, restoreRoot)
}

func RestoreDirectoryTo(path string, str string, header string, restoreRoot string) error {
	LogLoading("Building path registry")

	if !strings.HasPrefix(str, header) {
		return fmt.Errorf("package does not start with the expected header")
	}
	data := []byte(str[len(header):])
	if !bytes.HasPrefix(data, packageFormat) {
		PrintConfirm("fail")
		return fmt.Errorf("package has unsupported framing")
	}

	// Codec byte sits immediately after the format marker.
	codecOffset := len(packageFormat)
	if len(data) < codecOffset+1 {
		PrintConfirm("fail")
		return fmt.Errorf("package is missing the codec byte")
	}
	codec := PackageCodec(data[codecOffset])

	records, err := parsePackageRecords(data[codecOffset+1:])
	if err != nil {
		PrintConfirm("fail")
		return err
	}

	pathRegistry := make(map[string]map[int][]byte)

	for _, record := range records {
		archivedPath := record.archivedPath
		filePath, err := restorePathWithinRoot(restoreRoot, archivedPath)
		if err != nil {
			PrintConfirm("fail")
			return fmt.Errorf("invalid archived path %q: %w", archivedPath, err)
		}

		fmt.Print("\n" + filePath + "\n")

		raw, err := DecompressPayload([]byte(record.payload), codec)
		if err != nil {
			PrintConfirm("fail")
			return fmt.Errorf("decompress container %s: %w", archivedPath, err)
		}

		reader := bytes.NewReader(raw)
		numParts, err := readUint32LE(reader)
		if err != nil {
			PrintConfirm("fail")
			return fmt.Errorf("read part count for %s: %w", archivedPath, err)
		}

		if pathRegistry[filePath] == nil {
			pathRegistry[filePath] = make(map[int][]byte)
		}

		for i := uint32(0); i < numParts; i++ {
			partLen, err := readUint32LE(reader)
			if err != nil {
				PrintConfirm("fail")
				return fmt.Errorf("read part %d length for %s: %w", i, archivedPath, err)
			}
			if uint64(partLen) > uint64(reader.Len()) {
				PrintConfirm("fail")
				return fmt.Errorf("part %d length %d exceeds remaining payload %d", i, partLen, reader.Len())
			}
			partBytes := make([]byte, partLen)
			if _, err := io.ReadFull(reader, partBytes); err != nil {
				PrintConfirm("fail")
				return fmt.Errorf("read part %d bytes for %s: %w", i, archivedPath, err)
			}

			partData, err := RestoreFileRuntime(string(partBytes))
			if err != nil {
				PrintConfirm("fail")
				return fmt.Errorf("invalid payload for part%d.isp: %w", i, err)
			}

			if _, exists := pathRegistry[filePath][int(i)]; exists {
				PrintConfirm("fail")
				return fmt.Errorf("duplicate part index %d for %s", i, filePath)
			}
			pathRegistry[filePath][int(i)] = partData
		}
	}

	PrintConfirm("pass")
	LogLoading("Remerging file parts")

	mergedFiles := make(map[string][]byte)

	for filePath, parts := range pathRegistry {
		var indices []int
		for idx := range parts {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		if len(indices) == 0 || indices[0] != 0 {
			PrintConfirm("fail")
			return fmt.Errorf("file %s is missing part 0", filePath)
		}
		for i, idx := range indices {
			if idx != i {
				PrintConfirm("fail")
				return fmt.Errorf("file %s has non-contiguous part indexes", filePath)
			}
		}

		var merged strings.Builder
		for _, idx := range indices {
			merged.Write(parts[idx])
		}

		mergedFiles[filePath] = []byte(merged.String())
	}

	PrintConfirm("pass")
	LogLoading("Rebuilding directory")

	for filePath, data := range mergedFiles {
		dir := filepath.Dir(filePath)

		if err := os.MkdirAll(dir, 0755); err != nil {
			PrintConfirm("fail")
			return fmt.Errorf("Failed to create directory %s: %w", dir, err)
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			PrintConfirm("fail")
			return fmt.Errorf("Failed to write file %s: %w", filePath, err)
		}
	}

	PrintConfirm("pass")

	return nil
}

func containerPartIndex(name string) int {
	base := strings.TrimSuffix(name, ".isp")
	base = strings.TrimPrefix(base, "part")
	idx, err := strconv.Atoi(base)
	if err != nil {
		return 1 << 30
	}
	return idx
}

func writeUint32LE(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

func readUint32LE(r *bytes.Reader) (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

func parsePackageRecords(data []byte) ([]packageRecord, error) {
	reader := bytes.NewReader(data)
	records := make([]packageRecord, 0)
	for reader.Len() > 0 {
		var lengths [3]uint32
		if err := binary.Read(reader, binary.LittleEndian, &lengths); err != nil {
			return nil, fmt.Errorf("invalid binary package record header: %w", err)
		}

		fields := make([][]byte, len(lengths))
		for i, length := range lengths {
			if uint64(length) > uint64(reader.Len()) {
				return nil, fmt.Errorf("binary package field length %d exceeds remaining data", length)
			}
			fields[i] = make([]byte, int(length))
			if _, err := io.ReadFull(reader, fields[i]); err != nil {
				return nil, fmt.Errorf("read binary package field: %w", err)
			}
		}

		records = append(records, packageRecord{
			name:         string(fields[0]),
			archivedPath: string(fields[1]),
			payload:      string(fields[2]),
		})
	}
	return records, nil
}

func restorePathWithinRoot(restoreRoot string, archivedPath string) (string, error) {
	cleaned, err := cleanArchivePath(archivedPath)
	if err != nil {
		return "", err
	}

	root, err := filepath.Abs(restoreRoot)
	if err != nil {
		return "", fmt.Errorf("resolve restore root: %w", err)
	}
	destination := filepath.Join(root, cleaned)
	relative, err := filepath.Rel(root, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path escapes restore root")
	}

	return destination, nil
}

func cleanArchivePath(archivedPath string) (string, error) {
	normalized := strings.ReplaceAll(archivedPath, "\\", "/")

	if normalized == "" {
		return "", fmt.Errorf("path is empty")
	}

	if hasWindowsDriveLetter(normalized) {
		return "", fmt.Errorf("path must not contain a drive letter")
	}

	if strings.HasPrefix(normalized, "//") {
		return "", fmt.Errorf("path must not be a UNC path")
	}

	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("path must be relative")
	}

	native := filepath.FromSlash(normalized)
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" {
		return "", fmt.Errorf("path must be relative")
	}

	cleaned := filepath.Clean(native)
	if cleaned == "." || cleaned == ".." {
		return "", fmt.Errorf("path escapes restore root")
	}
	if strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes restore root")
	}

	return cleaned, nil
}

func isASCIIAlpha(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func hasWindowsDriveLetter(path string) bool {
	if len(path) < 2 {
		return false
	}
	if !isASCIIAlpha(path[0]) {
		return false
	}
	return path[1] == ':'
}

func SliceContents(path string, contId string) ([]string, error) {
	data, err := os.ReadFile(path + "/contents")
	if err != nil {
		return nil, err
	}

	byteSize := packetSize
	data = []byte(TextToInstructions(data))

	iterations := packagePartCount(len(data), byteSize)

	lzrs := make([]string, iterations)

	for i := 0; i < iterations; i++ {
		var slice []byte

		if (i + 1) < iterations {
			slice = data[i*byteSize : (i+1)*byteSize]
		} else {
			slice = data[i*byteSize:]
		}

		if err := os.WriteFile(path+"/part"+strconv.Itoa(i)+".isp~", slice, 0644); err != nil {
			return nil, err
		}

		lzrs[i] = path + "/part" + strconv.Itoa(i) + ".isp~"
	}

	if err := os.Remove(path + "/contents"); err != nil {
		return nil, err
	}

	return lzrs, nil
}

func packagePartCount(dataLength int, byteSize int) int {
	if dataLength == 0 {
		return 1
	}
	return (dataLength + byteSize - 1) / byteSize
}

func SealContainer(path string) error {
	dir, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	var files []string
	for _, f := range dir {
		if f.IsDir() {
			return fmt.Errorf("unexpected directory %s", f.Name())
		}
		if filepath.Ext(f.Name()) == ".isp" {
			continue
		}
		files = append(files, f.Name())
	}

	for _, f := range files {
		if err := os.Remove(filepath.Join(path, f)); err != nil {
			return err
		}
	}

	return nil
}

func BuildPackage(tree *Tree) error {
	if tree == nil {
		return fmt.Errorf("cannot build package from nil tree")
	}
	packageErr := MakePackage(tree)
	if packageErr != nil {
		return packageErr
	}
	tempPaths := tree.TempPaths
	if len(tempPaths) == 0 && tree.TempPath != "" {
		tempPaths = []string{tree.TempPath}
	}
	for _, tempPath := range tempPaths {
		if err := os.RemoveAll(tempPath); err != nil {
			return fmt.Errorf("remove temporary workspace %s: %w", tempPath, err)
		}
	}
	return nil
}

func MakePackage(tree *Tree) error {
	if tree == nil {
		return fmt.Errorf("cannot build package from nil tree")
	}
	basePath := tree.Path + "/" + tree.Name + ".lzr"

	// Slice, not map, so serialization is deterministic across runs.
	var contents [][]byte
	var filesToRemove []string

	for _, b := range tree.Containers {
		dir, err := os.ReadDir(b.TmpPath)
		if err != nil {
			return fmt.Errorf("read container %s: %w", b.TmpPath, err)
		}

		type partEntry struct {
			name string
			path string
			data []byte
		}
		var parts []partEntry

		for _, f := range dir {
			fPath := b.TmpPath + "/" + f.Name()
			if f.IsDir() {
				return fmt.Errorf("unexpected directory %s in container %s", f.Name(), b.TmpPath)
			}
			if filepath.Ext(f.Name()) != ".isp" {
				return fmt.Errorf("unexpected file %s in container %s", f.Name(), b.TmpPath)
			}

			data, err := os.ReadFile(fPath)
			if err != nil {
				return fmt.Errorf("read packaged part %s: %w", fPath, err)
			}
			parts = append(parts, partEntry{name: f.Name(), path: fPath, data: data})
		}

		sort.Slice(parts, func(i, j int) bool {
			return containerPartIndex(parts[i].name) < containerPartIndex(parts[j].name)
		})

		// Concatenate all parts of this container with 4-byte length
		// prefixes, then run the selected codec over the whole buffer.
		// This exposes cross-part redundancy to the downstream codec.
		var raw bytes.Buffer
		writeUint32LE(&raw, uint32(len(parts)))
		for _, p := range parts {
			bin := InstructionsToBytesRuntime(string(p.data))
			writeUint32LE(&raw, uint32(len(bin)))
			raw.Write(bin)
			filesToRemove = append(filesToRemove, p.path)
		}

		compressed, err := CompressPayload(raw.Bytes(), packagingCodec)
		if err != nil {
			return fmt.Errorf("compress container %s: %w", b.TmpPath, err)
		}

		relativePath := b.RelativePath
		if relativePath == "" {
			var err error
			relativePath, err = filepath.Rel(filepath.Join(tree.Path, tree.Name), b.FullPath)
			if err != nil {
				return fmt.Errorf("derive archive path: %w", err)
			}
			relativePath = filepath.ToSlash(relativePath)
		}
		relativePath, err = cleanArchivePath(relativePath)
		if err != nil {
			return fmt.Errorf("validate archive path: %w", err)
		}
		relativePath = filepath.ToSlash(relativePath)

		recordName := b.Name
		if recordName == "" {
			recordName = "_container"
		}
		contents = append(contents, encodePackageRecord(recordName, relativePath, compressed))
	}

	final := append([]byte(nil), packageFormat...)
	final = append(final, byte(packagingCodec))
	for _, b := range contents {
		final = append(final, b...)
	}

	header := MakeHeader(tree.Name+".lzr", len(final))
	writeData := append([]byte(header), final...)

	if err := os.WriteFile(basePath, []byte(writeData), 0644); err != nil {
		return fmt.Errorf("write package %s: %w", basePath, err)
	}
	for _, file := range filesToRemove {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("remove packaged part %s: %w", file, err)
		}
	}

	return nil
}

// CompressPayload applies the selected downstream codec to a container's
// bit-ops payload. The result is what gets written into the record's payload
// field. Codecs that are declared but not yet implemented return an error so
// the caller fails loudly rather than writing an undecodable archive.
func CompressPayload(data []byte, codec PackageCodec) ([]byte, error) {
	switch codec {
	case CodecRaw:
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	case CodecDeflate:
		var buf bytes.Buffer
		w, err := flate.NewWriter(&buf, flate.DefaultCompression)
		if err != nil {
			return nil, fmt.Errorf("init deflate writer: %w", err)
		}
		if _, err := w.Write(data); err != nil {
			w.Close()
			return nil, fmt.Errorf("deflate write: %w", err)
		}
		if err := w.Close(); err != nil {
			return nil, fmt.Errorf("deflate close: %w", err)
		}
		return buf.Bytes(), nil
	case CodecZstd:
		return nil, fmt.Errorf("codec zstd is reserved but not yet implemented")
	case CodecXz:
		return nil, fmt.Errorf("codec xz is reserved but not yet implemented")
	default:
		return nil, fmt.Errorf("unknown codec 0x%02x", byte(codec))
	}
}

// DecompressPayload reverses CompressPayload for the codec identified by the
// archive's codec byte.
func DecompressPayload(data []byte, codec PackageCodec) ([]byte, error) {
	switch codec {
	case CodecRaw:
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	case CodecDeflate:
		r := flate.NewReader(bytes.NewReader(data))
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("inflate payload: %w", err)
		}
		return out, nil
	case CodecZstd:
		return nil, fmt.Errorf("codec zstd is reserved but not yet implemented")
	case CodecXz:
		return nil, fmt.Errorf("codec xz is reserved but not yet implemented")
	default:
		return nil, fmt.Errorf("unknown codec 0x%02x", byte(codec))
	}
}

func firstDiffIndex(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// 2403 is a constant.
func MakeHeader(filename string, len int) string {
	str := laserheader + filename + laserseperator + strconv.Itoa(2403) + laserfooter + "\n"
	return str
}

func encodePackageRecord(name, archivedPath string, payload []byte) []byte {
	fields := [][]byte{[]byte(name), []byte(archivedPath), payload}
	framed := make([]byte, 12)
	for i, field := range fields {
		if uint64(len(field)) > uint64(^uint32(0)) {
			panic("package field exceeds uint32 length")
		}
		binary.LittleEndian.PutUint32(framed[i*4:], uint32(len(field)))
	}
	for _, field := range fields {
		framed = append(framed, field...)
	}
	return framed
}

func GetContextPath(path string, rootDirectoryPath string) (string, error) {
	if path == "" || rootDirectoryPath == "" {
		return "", fmt.Errorf("path and root directory are required")
	}
	root, err := filepath.Abs(rootDirectoryPath)
	if err != nil {
		return "", fmt.Errorf("resolve root directory: %w", err)
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve file path: %w", err)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("compute relative path: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path is not a file beneath the root directory")
	}
	return filepath.ToSlash(relative), nil
}

func GetContainerPath(rootName string, contextPath string) string {
	path := rootName + "." + strings.ReplaceAll(contextPath, "/", ".")
	path = strings.ReplaceAll(path, "\\", ".")
	return path
}
