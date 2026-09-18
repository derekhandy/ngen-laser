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
	"sort"
	"strconv"
	"strings"
)

type Instructions struct {
	guard *recursionGuard
}

func operandDigitLength() int {
	return operandLength
}

// parseMagnitude reads the magnitude value that follows a magnitude operand
// at position j in s. Returns the value and the number of digits consumed
// (1 or 2). A two-digit value is only recognized when it falls in the
// 10-31 range, so a lone "3" reads as 3 while "30" reads as 30 and "32"
// reads as 3 followed by a second glyph.
func parseMagnitude(s string, j int) (int, int, bool) {
	if j+1 >= len(s) {
		return 0, 0, false
	}
	d1 := s[j+1]
	if d1 < '1' || d1 > '9' {
		return 0, 0, false
	}
	if j+2 < len(s) {
		d2 := s[j+2]
		if d2 >= '0' && d2 <= '9' {
			combined := int(d1-'0')*10 + int(d2-'0')
			if combined >= 10 && combined <= 31 {
				return combined, 2, true
			}
		}
	}
	return int(d1 - '0'), 1, true
}

func NewInstructions() *Instructions {
	guard := NewGuard(50)
	return &Instructions{guard: &guard}
}

func NewInstructionlimit() int {
	return maxExpansionLimit
}

func SpendInstructionlimit(limit *int) bool {
	if limit == nil {
		return true
	}
	if *limit <= 0 {
		return false
	}
	*limit--
	return true
}

func (i *Instructions) PreInterpret(instructions string) string {
	modified := instructions

	if strings.Contains(instructions, "v") {
		for j := 0; j < len(instructions); j++ {
			p := string(instructions[j])
			if p == "v" {
				varStr := DetermineNextVariable(instructions, true)
				modified := instructions[:j] + varStr + instructions[j+1:]
				return modified
			}
		}
	}

	return modified
}

func (i *Instructions) Interpret(instructions string, iteration int, index *[]IndexEntry) string {
	limit := NewInstructionlimit()
	return i.InterpretWithlimit(instructions, iteration, index, &limit)
}

func (i *Instructions) InterpretWithlimit(instructions string, iteration int, index *[]IndexEntry, limit *int) string {
	if strings.Contains(instructions, "+") {
		return "+"
	}
	if len(instructions) > maxStringLength || !SpendInstructionlimit(limit) {
		return "+"
	}

	key := fmt.Sprintf("Interpret:%s", instructions)
	if !i.guard.Enter(key) {
		return instructions
	}
	defer i.guard.Exit(key)

	instructions = i.PreInterpret(instructions)

	modified := true
	for modified {
		if !SpendInstructionlimit(limit) {
			return "+"
		}
		modified = false
		for j := 0; j < len(instructions); j++ {
			if instructions[j] != '!' {
				continue
			}

			pre := instructions[:j]
			post := instructions[j+1:]

			if len(post) == 0 {
				continue
			}

			char := string(post[0])
			next := i.MendWithlimit(pre+post, char, j, index, limit)
			if next == instructions {
				return "+"
			}
			instructions = next
			modified = true
			break
		}

		if len(instructions) > maxStringLength {
			return "+"
		}
	}

	return instructions
}

func (i *Instructions) MendWithlimit(instructions, operand string, iteration int, index *[]IndexEntry, limit *int) string {
	if len(instructions) > maxStringLength || !SpendInstructionlimit(limit) {
		return "+"
	}

	key := fmt.Sprintf("Mend:%d:%d", iteration, len(instructions))

	if !i.guard.Enter(key) {
		return "+"
	}
	defer i.guard.Exit(key)

	j := iteration
	var segments [2]string
	segments[0] = instructions[:j+1]
	addition := 1

	switch operand {
	case "h":
		if j+3 <= len(instructions) {
			segments[1] = instructions[j+3:]
		} else {
			segments[1] = "+"
		}
		addition = 1
	case "k":
		if j+4 <= len(instructions) {
			segments[1] = instructions[j+4:]
		} else {
			segments[1] = "+"
		}
		addition = 1
	case "e", "o", "b":
		v, digits, ok := parseMagnitude(instructions, j)
		if !ok || j-v < 0 {
			segments[1] = "+"
			break
		}
		tailStart := j + 1 + digits + v
		if tailStart > len(instructions) {
			tailStart = len(instructions)
		}
		segments[1] = strconv.Itoa(v) + instructions[tailStart:]
		addition = 1 + digits
	case "t":
		if j+1 >= len(instructions) {
			segments[1] = "+"
			break
		}
		d := string(instructions[j+1])
		tailStart := j + 2 + operandDigitLength()
		if tailStart <= len(instructions) {
			segments[1] = d + instructions[tailStart:]
		} else {
			segments[1] = "+"
		}
		addition = 2
	case "f":
		if j+1 >= len(instructions) {
			segments[1] = "+"
			break
		}
		direction := string(instructions[j+1])
		tailStart := j + 2 + operandDigitLength()
		if tailStart <= len(instructions) {
			segments[1] = direction + instructions[tailStart:]
		} else {
			segments[1] = "+"
		}
		addition = 2
	case "x", "y", "z":
		v, digits, ok := parseMagnitude(instructions, j)
		if !ok {
			segments[1] = "+"
			break
		}
		_ = v
		tailStart := j + 1 + digits + operandDigitLength()
		if tailStart <= len(instructions) {
			segments[1] = instructions[j+1:j+1+digits] + instructions[tailStart:]
		} else {
			segments[1] = "+"
		}
		addition = 1 + digits
	case "r":
		if j+1 < len(instructions) {
			x := string(instructions[j+1])
			tailStart := j + 2 + operandDigitLength()
			if tailStart <= len(instructions) {
				segments[1] = x + instructions[tailStart:]
			} else {
				segments[1] = "+"
			}
		} else {
			segments[1] = "+"
		}
		addition = 2
	case "i":
		v, varStr, lookupStart, lookupEnd, ok := ParseIndexDefinition(instructions, j)
		if ok {
			lookup := instructions[lookupStart:lookupEnd]
			segmentsMod := instructions[lookupEnd:]
			*index = append(*index, IndexEntry{Lookup: lookup, Variable: varStr})
			segmentsMod = strings.Replace(segmentsMod, lookup, varStr, -1)
			lengthStr := strconv.Itoa(v)
			segments[1] = lengthStr + varStr + lookup + segmentsMod
			addition = len(lengthStr) + 2
		} else {
			segments[1] = "+"
		}
	case "m", "d":
		v, digits, ok := parseMagnitude(instructions, j)
		if !ok {
			segments[1] = "+"
			break
		}
		_ = v
		tail := j + 1 + digits + operandDigitLength()
		if tail <= len(instructions) {
			segments[1] = instructions[j+1:j+1+digits] + instructions[tail:]
		} else {
			segments[1] = "+"
		}
		addition = 1 + digits
	case "n":
		if j+1+operandDigitLength() <= len(instructions) {
			segments[1] = instructions[j+1+operandDigitLength():]
		} else {
			segments[1] = "+"
		}
		addition = 1
	case "g":
		if j+1+operandDigitLength() <= len(instructions) {
			segments[1] = instructions[j+1+operandDigitLength():]
		} else {
			segments[1] = "+"
		}
		addition = 1
	case "l":
		if j+1+operandDigitLength() <= len(instructions) {
			segments[1] = instructions[j+1+operandDigitLength():]
		} else {
			segments[1] = "+"
		}
		addition = 1
	case "s":
		if j+2 >= len(instructions) {
			segments[1] = "+"
			break
		}
		v, err := strconv.Atoi(string(instructions[j+1]))
		if err != nil || v <= 0 || j-v < 0 || j+2+v > len(instructions) {
			segments[1] = "+"
			break
		}
		before := instructions[j-v : j]
		after := instructions[j+2 : j+2+v]
		segments[0] = instructions[:j-v]
		segments[1] = after + instructions[j:j+2] + before + instructions[j+2+v:]
		addition = 2
	default:
		return "+"
	}

	appended := segments[0] + segments[1]
	if len(appended) > maxStringLength || !SpendInstructionlimit(limit) {
		return "+"
	}
	return i.InterpretWithlimit(appended, j+addition, index, limit)
}

func (i *Instructions) Devariablize(parsed string, iter int) string {
	lV := map[string]int{
		"A": 1, "B": 2, "C": 3, "D": 4, "E": 5, "F": 6, "G": 7, "H": 8, "I": 9, "J": 10,
		"K": 11, "L": 12, "M": 13, "N": 14, "O": 15, "P": 16, "Q": 17, "R": 18, "S": 19, "T": 20,
		"U": 21, "V": 22, "W": 23, "X": 24, "Y": 25, "Z": 26,
	}

	limit := maxExpansionLimit
	return i.IndexAllVariables(parsed, iter, lV, &limit)
}

func (i *Instructions) IndexAllVariables(parsed string, iteration int, lV map[string]int, limit *int) string {
	if len(parsed) > maxStringLength {
		return parsed[:maxStringLength]
	}
	if *limit <= 0 {
		return parsed
	}

	for iter := iteration; iter >= 1; iter-- {
		var varStr string
		for k, v := range lV {
			if v == iter {
				varStr = k
				break
			}
		}
		if strings.Contains(parsed, varStr) {
			for j := 0; j < len(parsed); j++ {
				if string(parsed[j]) == varStr {
					*limit--
					indexed := i.IndexVariable(parsed, j, true)
					if indexed == parsed {
						continue
					}
					return i.IndexAllVariables(indexed, iter, lV, limit)
				}
			}
		}
	}
	return parsed
}

func (i *Instructions) IndexVariable(s string, idx int, reverse bool) string {
	defIdx := idx
	_, varStr, _, _, ok := ParseIndexDefinition(s, defIdx)
	if !ok {
		defIdx, _, varStr, _, _, ok = ParseIndexDefinitionBeforeVariable(s, idx)
	}
	if !ok {
		return s
	}

	_, _, lookupStart, lookupEnd, ok := ParseIndexDefinition(s, defIdx)
	if !ok {
		return s
	}

	prefix := s[:defIdx]
	lookup := s[lookupStart:lookupEnd]
	tail := s[lookupEnd:]

	if !strings.Contains(tail, varStr) {
		return "+"
	}

	tail = strings.Replace(tail, varStr, lookup, -1)
	return prefix + lookup + tail
}

func ParseIndexDefinition(s string, idx int) (int, string, int, int, bool) {
	if idx < 0 || idx >= len(s) || s[idx] != 'i' {
		return 0, "", 0, 0, false
	}

	digitStart := idx + 1
	digitEnd := digitStart
	for digitEnd < len(s) && s[digitEnd] >= '0' && s[digitEnd] <= '9' {
		digitEnd++
	}
	if digitEnd == digitStart || digitEnd >= len(s) || !IsVariableByte(s[digitEnd]) {
		return 0, "", 0, 0, false
	}

	length, err := strconv.Atoi(s[digitStart:digitEnd])
	if err != nil || length <= 0 {
		return 0, "", 0, 0, false
	}

	lookupStart := digitEnd + 1
	lookupEnd := lookupStart + length
	if lookupEnd > len(s) {
		return 0, "", 0, 0, false
	}

	return length, string(s[digitEnd]), lookupStart, lookupEnd, true
}

func ParseIndexDefinitionBeforeVariable(s string, variableIdx int) (int, int, string, int, int, bool) {
	if variableIdx < 0 || variableIdx >= len(s) || !IsVariableByte(s[variableIdx]) {
		return 0, 0, "", 0, 0, false
	}

	defIdx := variableIdx - 1
	for defIdx >= 0 && s[defIdx] >= '0' && s[defIdx] <= '9' {
		defIdx--
	}
	if defIdx < 0 {
		return 0, 0, "", 0, 0, false
	}

	length, varStr, lookupStart, lookupEnd, ok := ParseIndexDefinition(s, defIdx)
	if !ok || lookupStart != variableIdx+1 {
		return 0, 0, "", 0, 0, false
	}

	return defIdx, length, varStr, lookupStart, lookupEnd, true
}

func (i *Instructions) Render(parsed string, iteration int, index *[]IndexEntry) string {
	limit := NewInstructionlimit()
	return i.RenderWithlimit(parsed, iteration, index, &limit)
}

func (i *Instructions) RenderWithlimit(parsed string, iteration int, index *[]IndexEntry, limit *int) string {
	if len(parsed) > maxStringLength || !SpendInstructionlimit(limit) {
		return "+"
	}

	key := fmt.Sprintf("Render:%d:%d", iteration, len(parsed))
	if !i.guard.Enter(key) {
		return "+"
	}
	defer i.guard.Exit(key)

	for j := iteration; j < len(parsed); j++ {
		p := string(parsed[j])
		if p == "-" || p == "," || p == "." || p == ";" {
			continue
		} else {
			return i.ReverseOperationWithlimit(parsed, p, j, index, limit)
		}
	}
	return parsed
}

func (i *Instructions) ReverseOperationWithlimit(instructions, p string, iteration int, index *[]IndexEntry, limit *int) string {
	if len(instructions) > maxStringLength || !SpendInstructionlimit(limit) {
		return "+"
	}

	key := fmt.Sprintf("ReverseOperation:%d:%d", iteration, len(instructions))
	if !i.guard.Enter(key) {
		return "+"
	}
	defer i.guard.Exit(key)

	j := iteration

	switch p {
	case "x", "y", "z":
		if j-operandDigitLength() >= 0 {
			v, digits, ok := parseMagnitude(instructions, j)
			if ok {
				transformed := i.Rotate(instructions, j, v, digits, true)
				return i.RenderWithlimit(transformed, j+1+digits+operandDigitLength(), index, limit)
			}
		}
	case "h":
		if j-1 >= 0 {
			return i.RenderWithlimit(i.Echo(instructions, iteration, 2, true), j+3, index, limit)
		}
	case "k":
		if j-1 >= 0 {
			return i.RenderWithlimit(i.Echo(instructions, iteration, 3, true), j+4, index, limit)
		}
	case "e":
		if j+1 < len(instructions) {
			v, digits, ok := parseMagnitude(instructions, j)
			if ok && j-v >= 0 {
				transformed := i.EchoVariable(instructions, j, v, digits, true)
				return i.RenderWithlimit(transformed, j+digits+v, index, limit)
			}
		}
	case "o":
		if j+1 < len(instructions) {
			v, digits, ok := parseMagnitude(instructions, j)
			if ok && j-1 >= 0 {
				transformed := i.EchoDigitVariable(instructions, j, v, digits, true)
				return i.RenderWithlimit(transformed, j+digits+v, index, limit)
			}
		}
	case "t":
		if j+1 < len(instructions) {
			return i.RenderWithlimit(i.Translate(instructions, iteration, true), j+1+operandDigitLength(), index, limit)
		}
	case "f":
		if j+1 < len(instructions) {
			return i.RenderWithlimit(i.Translate2(instructions, iteration, true), j+1+operandDigitLength(), index, limit)
		}
	case "r":
		if j+1 < len(instructions) {
			if j-operandDigitLength() >= 0 {
				return i.RenderWithlimit(i.Mirror(instructions, iteration, true), j+1+operandDigitLength(), index, limit)
			}
		}
	case "i":
		_, _, _, _, ok := ParseIndexDefinition(instructions, j)
		if ok {
			return i.RenderWithlimit(i.IndexVariable(instructions, iteration, true), j, index, limit)
		}
	case "m":
		if j+1 < len(instructions) {
			v, digits, ok := parseMagnitude(instructions, j)
			if ok && j-operandDigitLength() >= 0 {
				transformed := i.Multiply(instructions, j, false, true, v, digits)
				return i.RenderWithlimit(transformed, j+1+digits+operandDigitLength(), index, limit)
			}
		}
	case "d":
		if j+1 < len(instructions) {
			v, digits, ok := parseMagnitude(instructions, j)
			if ok && j-operandDigitLength() >= 0 {
				transformed := i.Multiply(instructions, j, true, true, v, digits)
				return i.RenderWithlimit(transformed, j+1+digits+operandDigitLength(), index, limit)
			}
		}
	case "n":
		if j-operandDigitLength() >= 0 {
			return i.RenderWithlimit(i.Invert(instructions, iteration, true), j+operandDigitLength(), index, limit)
		}
	case "g":
		if j-operandDigitLength() >= 0 {
			return i.RenderWithlimit(i.Sort(instructions, iteration, true, true), j+operandDigitLength(), index, limit)
		}
	case "l":
		if j-operandDigitLength() >= 0 {
			return i.RenderWithlimit(i.Sort(instructions, iteration, false, true), j+operandDigitLength(), index, limit)
		}
	case "b":
		if j+1 < len(instructions) {
			v, digits, ok := parseMagnitude(instructions, j)
			if ok && j-v >= 0 {
				transformed := i.ReverseVariable(instructions, j, v, digits, true)
				return i.RenderWithlimit(transformed, j+digits+v, index, limit)
			}
		}
	case "s":
		if j+1 < len(instructions) {
			v, err := strconv.Atoi(string(instructions[j+1]))
			if err == nil && v > 0 && j-v >= 0 && j+2+v <= len(instructions) {
				before := instructions[j-v : j]
				after := instructions[j+2 : j+2+v]
				swapped := instructions[:j-v] + after + before + instructions[j+2+v:]
				return i.RenderWithlimit(swapped, j+v, index, limit)
			}
		}
	}

	return i.RenderWithlimit(instructions, j+1, index, limit)
}

func (i *Instructions) VerifyContinuity(rendered, original string) bool {
	return rendered == original
}

func (i *Instructions) Echo(s string, index int, amount int, reverse bool) string {
	prefix := s[:index]
	segment := s[index+1:]
	digit := ""
	for e := 0; e < amount; e++ {
		digit += string(s[index-1])
	}
	return prefix + digit + segment
}

// EchoVariable reverses an `eN` operand during render. The operand sits at
// s[index], the magnitude spans s[index+1:index+1+digits], and the v glyphs
// immediately before the operand are re-inserted in place of the operand
// and its magnitude. The tail follows at s[index+1+digits:].
func (i *Instructions) EchoVariable(s string, index int, v int, digits int, reverse bool) string {
	if v <= 0 || index-v < 0 || index+1+digits > len(s) {
		return "+"
	}
	prefix := s[:index]
	segment := s[index+1+digits:]
	digitsSlice := s[index-v : index]
	return prefix + digitsSlice + segment
}

// EchoDigitVariable reverses an `oN` operand during render. The operand sits
// at s[index], the magnitude spans s[index+1:index+1+digits], and the glyph
// at s[index-1] is repeated v times in place of the operand and its
// magnitude. The tail follows at s[index+1+digits:].
func (i *Instructions) EchoDigitVariable(s string, index int, v int, digits int, reverse bool) string {
	if v <= 0 || index-1 < 0 || index+1+digits > len(s) {
		return "+"
	}
	if string(s[index-1]) == "o" {
		return "+"
	}
	prefix := s[:index]
	segment := s[index+1+digits:]
	digitsSlice := ""
	for e := 0; e < v; e++ {
		digitsSlice += string(s[index-1])
	}
	return prefix + digitsSlice + segment
}

// ReverseVariable reverses a `bN` operand during render. The operand sits at
// s[index], the magnitude spans s[index+1:index+1+digits], and the v glyphs
// immediately before the operand are reversed in place of the operand and
// its magnitude. The tail follows at s[index+1+digits:].
func (i *Instructions) ReverseVariable(s string, index int, v int, digits int, reverse bool) string {
	if v <= 0 || index-v < 0 || index+1+digits > len(s) {
		return "+"
	}
	prefix := s[:index]
	segment := s[index+1+digits:]
	digitsSlice := s[index-v : index]
	reversed := ""
	for i := len(digitsSlice) - 1; i >= 0; i-- {
		reversed += string(digitsSlice[i])
	}
	return prefix + reversed + segment
}

func (i *Instructions) Sort(s string, index int, descending bool, reverse bool) string {
	prefix := s[:index]
	segment := s[index+1:]
	digits := s[index-operandDigitLength() : index]
	glyphs := strings.Builder{}
	glyphs.Grow(len(digits))

	for lineStart := 0; lineStart < len(digits); lineStart += lineLength {
		line := []rune(digits[lineStart : lineStart+lineLength])
		ints := make([]int, len(line))

		for j := range line {
			value, err := i.GlyphToInt(string(line[j]))
			if err != nil {
				return "+"
			}
			ints[j] = value
		}

		if descending {
			sort.Slice(ints, func(i, j int) bool {
				return ints[i] > ints[j]
			})
		} else {
			sort.Slice(ints, func(i, j int) bool {
				return ints[i] < ints[j]
			})
		}

		for k := range ints {
			glyph, err := i.IntToGlyph(ints[k])
			if err != nil {
				return "+"
			}
			glyphs.WriteString(glyph)
		}
	}

	return prefix + glyphs.String() + segment
}

func (i *Instructions) translateCoordinateLine(line string, cV map[string]int, axis int, diff int) (string, bool) {
	if len(line) != lineLength {
		return "", false
	}

	out := []byte(line)
	for coordinateStart := 0; coordinateStart < lineLength; coordinateStart += coordinateLength {
		srcIdx := coordinateStart + axis
		sV, ok := cV[string(line[srcIdx])]
		if !ok {
			return "", false
		}
		nV := sV + diff
		found := ""
		for key, value := range cV {
			if value == nV {
				found = key
				break
			}
		}
		if found == "" {
			return "", false
		}
		out[srcIdx] = found[0]
	}

	return string(out), true
}

func (i *Instructions) translateOperandLines(s string, index int, cV map[string]int, axis int, diff int) (string, bool) {
	digits := s[index-operandDigitLength() : index]
	var segment strings.Builder
	segment.Grow(len(digits))

	for lineStart := 0; lineStart < len(digits); lineStart += lineLength {
		line, ok := i.translateCoordinateLine(digits[lineStart:lineStart+lineLength], cV, axis, diff)
		if !ok {
			return "", false
		}
		segment.WriteString(line)
	}

	return segment.String(), true
}

func (i *Instructions) Translate(s string, index int, reverse bool) string {
	cV := map[string]int{
		";": 1,
		".": 2,
		",": 3,
		"-": 4,
	}

	prefix := s[:index]
	suffix := ""
	if index+2 < len(s) {
		suffix = s[index+2:]
	}
	segment := ""
	if index < operandDigitLength() || index+1 >= len(s) {
		return "+"
	}
	switch string(s[index+1]) {
	case "u":
		newValues, ok := i.translateOperandLines(s, index, cV, 1, 1)
		if !ok {
			return "+"
		}
		segment = newValues
	case "d":
		newValues, ok := i.translateOperandLines(s, index, cV, 1, -1)
		if !ok {
			return "+"
		}
		segment = newValues
	case "r":
		newValues, ok := i.translateOperandLines(s, index, cV, 0, 1)
		if !ok {
			return "+"
		}
		segment = newValues
	case "l":
		newValues, ok := i.translateOperandLines(s, index, cV, 0, -1)
		if !ok {
			return "+"
		}
		segment = newValues
	case "f":
		newValues, ok := i.translateOperandLines(s, index, cV, 2, 1)
		if !ok {
			return "+"
		}
		segment = newValues
	case "b":
		newValues, ok := i.translateOperandLines(s, index, cV, 2, -1)
		if !ok {
			return "+"
		}
		segment = newValues
	default:
		return "+"
	}

	return prefix + segment + suffix
}

func (i *Instructions) Translate2(s string, index int, reverse bool) string {
	cV := map[string]int{
		";": 1,
		".": 2,
		",": 3,
		"-": 4,
	}

	prefix := s[:index]
	suffix := ""
	if index+2 < len(s) {
		suffix = s[index+2:]
	}
	segment := ""
	if index < operandDigitLength() || index+1 >= len(s) {
		return "+"
	}
	switch string(s[index+1]) {
	case "u":
		newValues, ok := i.translateOperandLines(s, index, cV, 1, 2)
		if !ok {
			return "+"
		}
		segment = newValues
	case "d":
		newValues, ok := i.translateOperandLines(s, index, cV, 1, -2)
		if !ok {
			return "+"
		}
		segment = newValues
	case "r":
		newValues, ok := i.translateOperandLines(s, index, cV, 0, 2)
		if !ok {
			return "+"
		}
		segment = newValues
	case "l":
		newValues, ok := i.translateOperandLines(s, index, cV, 0, -2)
		if !ok {
			return "+"
		}
		segment = newValues
	case "f":
		newValues, ok := i.translateOperandLines(s, index, cV, 2, 2)
		if !ok {
			return "+"
		}
		segment = newValues
	case "b":
		newValues, ok := i.translateOperandLines(s, index, cV, 2, -2)
		if !ok {
			return "+"
		}
		segment = newValues
	default:
		return "+"
	}

	return prefix + segment + suffix
}

// Rotate reverses an `xN`, `yN`, or `zN` operand during render. The operand
// sits at s[index], the magnitude spans s[index+1:index+1+digits], the
// source vector is the operandDigitLength() glyphs immediately before the
// operand, and the tail begins at s[index+1+digits+operandDigitLength():].
func (i *Instructions) Rotate(s string, index int, v int, digits int, reverse bool) string {
	cV := map[string]int{
		";": 1,
		".": 2,
		",": 3,
		"-": 4,
	}

	if index-operandDigitLength() < 0 || index+1+digits+operandDigitLength() > len(s) {
		return "+"
	}

	prefix := s[:index]
	segment := s[index+1+digits+operandDigitLength():]

	vector := s[index-operandDigitLength() : index]
	axis := string(s[index])

	var rotated strings.Builder
	rotated.Grow(len(vector))
	for lineStart := 0; lineStart < len(vector); lineStart += lineLength {
		if lineStart+lineLength > len(vector) {
			return "+"
		}
		switch axis {
		case "x":
			segment = i.RotateCoordinates(vector[lineStart:lineStart+lineLength], "x", cV)
		case "y":
			segment = i.RotateCoordinates(vector[lineStart:lineStart+lineLength], "y", cV)
		case "z":
			segment = i.RotateCoordinates(vector[lineStart:lineStart+lineLength], "z", cV)
		default:
			return "+"
		}
		if len(segment) != lineLength {
			return "+"
		}
		rotated.WriteString(segment)
	}

	return prefix + rotated.String() + segment
}

func (i *Instructions) RotateCoordinates(vector, axis string, cV map[string]int) string {
	newValues := ""

	for j := 0; j < lineLength/coordinateLength; j++ {
		sK := [3]string{
			string(vector[0+(j*3)]),
			string(vector[1+(j*3)]),
			string(vector[2+(j*3)]),
		}
		sV := Vector3Int{
			X: cV[sK[0]],
			Y: cV[sK[1]],
			Z: cV[sK[2]],
		}

		var rV Vector3Int
		switch axis {
		case "x":
			rV.X = sV.X
			rV.Y = sV.Z
			rV.Z = i.Negative(sV.Y)
		case "y":
			rV.X = i.Negative(sV.Z)
			rV.Y = sV.Y
			rV.Z = sV.X
		case "z":
			rV.X = i.Negative(sV.Y)
			rV.Y = sV.X
			rV.Z = sV.Z
		}

		for k, v := range cV {
			if v == rV.X {
				newValues += k
				break
			}
		}
		for k, v := range cV {
			if v == rV.Y {
				newValues += k
				break
			}
		}
		for k, v := range cV {
			if v == rV.Z {
				newValues += k
				break
			}
		}
	}

	return newValues
}

func (i *Instructions) Mirror(s string, index int, reverse bool) string {
	cV := map[string]int{
		";": 1,
		".": 2,
		",": 3,
		"-": 4,
	}

	prefix := s[:index]
	suffix := ""
	if index+2 < len(s) {
		suffix = s[index+2:]
	}
	segment := ""

	vector := s[index-operandDigitLength() : index]
	axis := string(s[index+1])

	var mirrored strings.Builder
	mirrored.Grow(len(vector))
	for coordinateStart := 0; coordinateStart < len(vector); coordinateStart += coordinateLength {
		switch axis {
		case "x", "y", "z":
			segment = i.MirrorCoordinates(vector[coordinateStart:coordinateStart+coordinateLength], axis, cV)
		default:
			return "+"
		}
		if len(segment) != coordinateLength {
			return "+"
		}
		mirrored.WriteString(segment)
	}

	return prefix + mirrored.String() + suffix
}

func (i *Instructions) MirrorCoordinates(vector, axis string, cV map[string]int) string {
	sK := [3]string{
		string(vector[0]),
		string(vector[1]),
		string(vector[2]),
	}
	sV := Vector3Int{
		X: cV[sK[0]],
		Y: cV[sK[1]],
		Z: cV[sK[2]],
	}
	var rV Vector3Int
	switch axis {
	case "x":
		rV.X = i.Negative(sV.X)
		rV.Y = sV.Z
		rV.Z = sV.Y
	case "y":
		rV.X = sV.Z
		rV.Y = i.Negative(sV.Y)
		rV.Z = sV.X
	case "z":
		rV.X = sV.Y
		rV.Y = sV.X
		rV.Z = i.Negative(sV.Z)
	}

	var result strings.Builder
	for k, v := range cV {
		if v == rV.X {
			result.WriteString(k)
			break
		}
	}
	for k, v := range cV {
		if v == rV.Y {
			result.WriteString(k)
			break
		}
	}
	for k, v := range cV {
		if v == rV.Z {
			result.WriteString(k)
			break
		}
	}

	return result.String()
}

// Multiply reverses an `mN` or `dN` operand during render. The operand sits
// at s[index], the magnitude spans s[index+1:index+1+digits], the source
// vector is the operandDigitLength() glyphs immediately before the operand,
// and the tail begins at s[index+1+digits+operandDigitLength():]. When
// shouldInvert is true, the multiplier is 1/v (the `d` case).
func (i *Instructions) Multiply(s string, index int, shouldInvert bool, reverse bool, v int, digits int) string {
	cV := map[string]int{
		";": -2,
		".": -1,
		",": 1,
		"-": 2,
	}

	if v <= 0 || index-operandDigitLength() < 0 || index+1+digits+operandDigitLength() > len(s) {
		return "+"
	}

	prefix := s[:index]
	segment := s[index+1+digits+operandDigitLength():]

	m := float32(v)
	if shouldInvert {
		m = 1 / m
	}

	newValues := i.MultiplyCoordinates(s, index, cV, m)
	return prefix + newValues + segment
}

func (i *Instructions) MultiplyCoordinates(s string, index int, cV map[string]int, m float32) string {
	var result strings.Builder
	result.Grow(operandDigitLength())
	for coordinateStart := index - operandDigitLength(); coordinateStart < index; coordinateStart += coordinateLength {
		sK := [3]string{
			string(s[coordinateStart]),
			string(s[coordinateStart+1]),
			string(s[coordinateStart+2]),
		}
		sV := Vector3{
			X: float32(cV[sK[0]]),
			Y: float32(cV[sK[1]]),
			Z: float32(cV[sK[2]]),
		}
		nV := Vector3Int{
			X: int(sV.X * m),
			Y: int(sV.Y * m),
			Z: int(sV.Z * m),
		}

		for k, v := range cV {
			if v == nV.X {
				result.WriteString(k)
				break
			}
		}
		for k, v := range cV {
			if v == nV.Y {
				result.WriteString(k)
				break
			}
		}
		for k, v := range cV {
			if v == nV.Z {
				result.WriteString(k)
				break
			}
		}
	}

	return result.String()
}

func (i *Instructions) Invert(s string, index int, reverse bool) string {
	cV := map[string]int{
		";": -2,
		".": -1,
		",": 1,
		"-": 2,
	}

	prefix := s[:index]
	suffix := ""
	if index+1 < len(s) {
		suffix = s[index+1:]
	}

	var newValues strings.Builder
	newValues.Grow(operandDigitLength())
	for coordinateStart := index - operandDigitLength(); coordinateStart < index; coordinateStart += coordinateLength {
		sK := [3]string{
			string(s[coordinateStart]),
			string(s[coordinateStart+1]),
			string(s[coordinateStart+2]),
		}
		sV := Vector3{
			X: float32(cV[sK[0]]),
			Y: float32(cV[sK[1]]),
			Z: float32(cV[sK[2]]),
		}
		nV := Vector3Int{
			X: int(sV.X * -1),
			Y: int(sV.Y * -1),
			Z: int(sV.Z * -1),
		}

		for k, v := range cV {
			if v == nV.X {
				newValues.WriteString(k)
				break
			}
		}
		for k, v := range cV {
			if v == nV.Y {
				newValues.WriteString(k)
				break
			}
		}
		for k, v := range cV {
			if v == nV.Z {
				newValues.WriteString(k)
				break
			}
		}
	}

	return prefix + newValues.String() + suffix
}

func CountToVariableString(count int) string {
	if count < 0 {
		return ""
	}

	var result strings.Builder

	for count >= 0 {
		result.WriteByte(byte('A' + count%variableTokenCount))
		count = count/variableTokenCount - 1
		if count < 0 {
			break
		}
	}

	runes := []rune(result.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}

func DetermineNextVariable(instructions string, subtract bool) string {
	count := strings.Count(instructions, "i")
	if subtract {
		count = count - 1
	}
	return CountToVariableString(count)
}

func DeterminePreviousVariable(instructions string) string {
	for i := 0; i < variableTokenCount; i++ {
		token := TokenFromIndex(i)
		if !strings.Contains(instructions, token) {
			return token
		}
	}
	return "A"
}

func (i *Instructions) Negative(value int) int {
	return 5 + (value * -1)
}

func (i *Instructions) GlyphToInt(glyph string) (int, error) {
	switch glyph {
	case "-":
		return 4, nil
	case ",":
		return 3, nil
	case ".":
		return 2, nil
	case ";":
		return 1, nil
	default:
		return -1, errors.New("invalid glyph")
	}
}

func (i *Instructions) IntToGlyph(index int) (string, error) {
	switch index {
	case 4:
		return "-", nil
	case 3:
		return ",", nil
	case 2:
		return ".", nil
	case 1:
		return ";", nil
	default:
		return "", errors.New("invalid index")
	}
}
