package main

import (
	"bytes"
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

var packageFormat = []byte("LASER-PACKAGE-V3\x00")

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
	records, err := parsePackageRecords(data[len(packageFormat):])
	if err != nil {
		PrintConfirm("fail")
		return err
	}

	pathRegistry := make(map[string]map[int][]byte)

	for _, record := range records {
		name := record.name
		archivedPath := record.archivedPath
		filePath, err := restorePathWithinRoot(restoreRoot, archivedPath)
		if err != nil {
			PrintConfirm("fail")
			return fmt.Errorf("invalid archived path %q: %w", archivedPath, err)
		}

		fmt.Print("\n" + filePath + "\n")

		data, err := RestoreFileRuntime(record.payload)
		if err != nil {
			PrintConfirm("fail")
			return fmt.Errorf("invalid payload for %s: %w", name, err)
		}

		cleanName := strings.TrimPrefix(name, "part")
		cleanName = strings.TrimSuffix(cleanName, ".isp")

		idx, err := strconv.Atoi(cleanName)
		if err != nil || idx < 0 {
			PrintConfirm("fail")
			return fmt.Errorf("index parsing error on %s", name)
		}

		if pathRegistry[filePath] == nil {
			pathRegistry[filePath] = make(map[int][]byte)
		}
		if _, exists := pathRegistry[filePath][idx]; exists {
			PrintConfirm("fail")
			return fmt.Errorf("duplicate part index %d for %s", idx, filePath)
		}

		pathRegistry[filePath][idx] = data
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
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			PrintConfirm("fail")
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
	}

	PrintConfirm("pass")

	return nil
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

func hasWindowsDriveLetter(path string) bool {
	if len(path) < 2 {
		return false
	}
	c := path[0]
	if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
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
	basePath := tree.Path + "/" + tree.Name + ".lzr"

	contents := map[int][]byte{}
	var filesToRemove []string

	count := 0
	for _, b := range tree.Containers {
		dir, err := os.ReadDir(b.TmpPath)
		if err != nil {
			return fmt.Errorf("read container %s: %w", b.TmpPath, err)
		}

		var files []string
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

			bin := InstructionsToBytesRuntime(string(data))

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

			cont := encodePackageRecord(f.Name(), relativePath, bin)

			contents[count] = cont
			count++

			files = append(files, fPath)
		}

		filesToRemove = append(filesToRemove, files...)
	}

	final := append([]byte(nil), packageFormat...)

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
