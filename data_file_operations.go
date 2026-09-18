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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

func ContainsAlphabetic(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func TextToBinary(text string) string {
	data := []byte(text)

	var bits strings.Builder
	bits.Grow(len(data) * 8)

	for _, b := range data {
		fmt.Fprintf(&bits, "%08b", b)
	}

	return bits.String()
}

func BinaryToInstructions(data string) []Vector3 {
	points := len(data) / 6
	if len(data)%6 != 0 {
		points++
	}

	instructions := make([]Vector3, points)
	for i := 0; i < points; i++ {
		start := i * 6
		end := start + 6
		if end > len(data) {
			end = len(data)
		}

		p := data[start:end]
		if len(p) < 6 {
			p = p + strings.Repeat("0", 6-len(p))
		}

		x := p[0:2]
		y := p[2:4]
		z := p[4:6]

		instructions[i] = Vector3{
			X: FloatFromBinary(x),
			Y: FloatFromBinary(y),
			Z: FloatFromBinary(z),
		}
	}

	return instructions
}

func BinaryToText(text string) string {
	text = strings.TrimSpace(text)

	if len(text) == 0 {
		return ""
	}

	if padding := len(text) % 8; padding != 0 {
		remainder := text[len(text)-padding:]
		if strings.IndexByte(remainder, '1') == -1 {
			text = text[:len(text)-padding]
		} else {
			text = text + strings.Repeat("0", 8-padding)
		}
	}

	bytes := make([]byte, len(text)/8)
	for i := 0; i < len(bytes); i++ {
		start := i * 8
		end := start + 8
		if end > len(text) {
			end = len(text)
		}

		byteStr := text[start:end]
		if len(byteStr) < 8 {
			byteStr = byteStr + strings.Repeat("0", 8-len(byteStr))
		}

		val, _ := strconv.ParseUint(byteStr, 2, 8)
		bytes[i] = byte(val)
	}

	return string(bytes)
}

func InstructionsToString(instructions []Vector3) string {
	var sb strings.Builder

	for _, v := range instructions {
		sb.WriteString(StringFromFloat(v.X))
		sb.WriteString(StringFromFloat(v.Y))
		sb.WriteString(StringFromFloat(v.Z))
	}

	return sb.String()
}

func TextToInstructions(data []byte) string {
	binary := TextToBinary(string(data))
	instructions := InstructionsToString(BinaryToInstructions(binary))
	return instructions
}

func StringToInstructions(text string) []Vector3 {
	instructions := make([]Vector3, len(text)/3)
	for i := 0; i < len(instructions); i++ {
		start := i * 3
		instructions[i] = Vector3{
			X: FloatFromString(text[start : start+1]),
			Y: FloatFromString(text[start+1 : start+2]),
			Z: FloatFromString(text[start+2 : start+3]),
		}
	}

	return instructions
}

func InstructionsToBinary(instructions []Vector3) string {
	var sb strings.Builder

	for _, v := range instructions {
		sb.WriteString(BinaryFromFloat(v.X))
		sb.WriteString(BinaryFromFloat(v.Y))
		sb.WriteString(BinaryFromFloat(v.Z))
	}

	return sb.String()
}

func BinaryFromFloat(v float32) string {
	if v > 0.5 {
		return "00"
	} else if v > 0 {
		return "01"
	} else if v > -0.5 {
		return "10"
	} else {
		return "11"
	}
}

func FloatFromString(v string) float32 {
	switch v {
	case "-":
		return 0.75
	case ",":
		return 0.25
	case ".":
		return -0.25
	default:
		return -0.75
	}
}

func FloatFromBinary(v string) float32 {
	switch v {
	case "00":
		return 0.75
	case "01":
		return 0.25
	case "10":
		return -0.25
	default:
		return -0.75
	}
}

func StringFromFloat(v float32) string {
	if v > 0.5 {
		return "-"
	} else if v > 0 {
		return ","
	} else if v > -0.5 {
		return "."
	} else {
		return ";"
	}
}

func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create network directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	cleanup = false
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}
