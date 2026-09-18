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
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

type SolveResult struct {
	Original           string
	BestString         string
	BestScore          float32
	BestNetwork        string
	OriginalPackedSize int
	BestPackedSize     int
	Rounds             int
	Improvements       int
	Maximal            bool
}

type SolveTarget struct {
	Label        string
	SourcePath   string
	Instructions string
}

var bitOpsBinaryMagic = []byte{'G', 'B', 'O', 'P'}

func InstructionsToBytesRuntime(input string) []byte {
	bitOperands := GlyphsToBitsPreservingOperands(strings.TrimSpace(input))
	encoded := EncodeBitOpsBinary([]byte(bitOperands))

	return encoded
}

func EncodeBitOpsBinary(input []byte) []byte {
	var out bytes.Buffer
	out.Write(bitOpsBinaryMagic)

	for _, token := range PacketTokensFromBitOperands(input) {
		WritePacketToken(&out, token)
	}

	return out.Bytes()
}

func RestoreFileRuntime(input string) ([]byte, error) {
	bitOperands, err := DecodeBitOpsBinary([]byte(input))
	if err != nil {
		return nil, fmt.Errorf("decode file payload: %w", err)
	}

	restored, _, _, err := RestoreBitOperandsToData(bitOperands)
	if err != nil {
		return nil, fmt.Errorf("restore file payload: %w", err)
	}

	return restored, nil
}

func RestoreBitOperandsToData(bitOperands []byte) ([]byte, string, string, error) {
	if len(bitOperands) == 0 {
		return []byte{}, "", "", nil
	}

	glyphOperands, err := BitsAndOperandsToGlyphs(strings.TrimSpace(string(bitOperands)))
	if err != nil {
		return nil, "", "", err
	}

	rendered := NewICommands().ReturnRendered(glyphOperands)
	if rendered == "" || rendered == "+" {
		return nil, glyphOperands, rendered, fmt.Errorf("failed to render glyph+operand instructions")
	}
	if !ContainsOnlyGlyphs(rendered) {
		return nil, glyphOperands, rendered, fmt.Errorf("rendered instructions still contain operands near %s", FirstOperandContext(rendered))
	}
	if len(rendered)%3 != 0 {
		return nil, glyphOperands, rendered, fmt.Errorf("rendered glyph length %d is not divisible by 3", len(rendered))
	}

	text := BinaryToText(InstructionsToBinary(StringToInstructions(rendered)))
	return []byte(text), glyphOperands, rendered, nil
}

func DecodeBitOpsBinary(encoded []byte) ([]byte, error) {
	if !bytes.HasPrefix(encoded, bitOpsBinaryMagic) {
		return nil, fmt.Errorf("binary bitops input has invalid header")
	}

	reader := bytes.NewReader(encoded[len(bitOpsBinaryMagic):])
	var out bytes.Buffer

	for reader.Len() > 0 {
		token, err := reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("failed to read binary bitops token: %w", err)
		}
		if token < compactOperandFixedBase {
			bitLen := uint64(token) + 1
			payloadLen := PackedBitRunLength(bitLen)
			if payloadLen > uint64(reader.Len()) {
				return nil, fmt.Errorf("binary bitops bit run length %d exceeds remaining payload %d", payloadLen, reader.Len())
			}
			payload := make([]byte, int(payloadLen))
			if _, err := io.ReadFull(reader, payload); err != nil {
				return nil, fmt.Errorf("failed to read binary bitops bit run: %w", err)
			}
			if HasNonZeroBitPadding(payload, bitLen) {
				return nil, fmt.Errorf("binary bitops bit run has non-zero padding bits")
			}
			out.Write(UnPackBitRun(payload, bitLen))
			continue
		}

		if err := DecodeCompactOperandToken(reader, &out, token); err != nil {
			return nil, err
		}
	}

	return out.Bytes(), nil
}

func WriteCompactBitRun(out *bytes.Buffer, bits []byte) {
	for len(bits) > 0 {
		chunkLen := len(bits)
		if chunkLen > compactBitRunMax {
			chunkLen = compactBitRunMax
		}
		out.WriteByte(byte(chunkLen - 1))
		out.Write(PackBitRun(bits[:chunkLen]))
		bits = bits[chunkLen:]
	}
}

func DecodeCompactOperandToken(reader *bytes.Reader, out *bytes.Buffer, token byte) error {
	parsed, err := ReadPacketOperandToken(reader, token)
	if err != nil {
		return err
	}
	WritePacketTokenText(out, parsed)
	return nil
}

func WriteCompactRawOperandRun(out *bytes.Buffer, raw []byte) {
	if len(raw) == 0 {
		return
	}
	out.WriteByte(compactOperandRawRun)
	var lengthBuf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(lengthBuf[:], uint64(len(raw)))
	out.Write(lengthBuf[:n])
	out.Write(raw)
}

func CompactMagnitudeOperandIndex(op byte) int {
	switch op {
	case 'e':
		return 0
	case 'm':
		return 1
	case 'd':
		return 2
	case 'b':
		return 3
	case 'o':
		return 4
	case 'x':
		return 5
	case 'y':
		return 6
	case 'z':
		return 7
	default:
		return -1
	}
}

func DirectionIndex(dir byte) int {
	for idx, class := range directionClasses {
		if len(class) == 1 && class[0] == dir {
			return idx
		}
	}
	return -1
}

func AxisIndex(axis byte) int {
	for idx, class := range axisClasses {
		if len(class) == 1 && class[0] == axis {
			return idx
		}
	}
	return -1
}

func OperandTokenEnd(s string, idx int) int {
	if idx >= len(s) {
		return idx
	}

	switch s[idx] {
	case 'h', 'k', 'n', 'g', 'l':
		return idx + 1
	case 'e', 'm', 'd', 'b', 'o', 'x', 'y', 'z':
		if idx+1 >= len(s) {
			return idx + 1
		}
		d1 := s[idx+1]
		if d1 < '1' || d1 > '9' {
			return idx + 1
		}
		if idx+2 < len(s) {
			d2 := s[idx+2]
			if d2 >= '0' && d2 <= '9' {
				combined := int(d1-'0')*10 + int(d2-'0')
				if combined >= 10 && combined <= 31 {
					return idx + 3
				}
			}
		}
		return idx + 2
	case 's':
		if idx+1 < len(s) && s[idx+1] >= '1' && s[idx+1] <= '9' {
			return idx + 2
		}
		return idx + 1
	case 't', 'f':
		if idx+1 < len(s) && IsDirectionByte(s[idx+1]) {
			return idx + 2
		}
		return idx + 1
	case 'r':
		if idx+1 < len(s) && IsAxisByte(s[idx+1]) {
			return idx + 2
		}
		return idx + 1
	case 'i':
		end := ConsumeDigits(s, idx+1)
		if end < len(s) && IsVariableByte(s[end]) {
			end++
		}
		if end == idx+1 {
			return idx + 1
		}
		return end
	default:
		return idx + 1
	}
}

func PackBitRun(bits []byte) []byte {
	packed := make([]byte, PackedBitRunLength(uint64(len(bits))))
	for idx, bit := range bits {
		if bit == '1' {
			packed[idx/8] |= 1 << uint(7-(idx%8))
		}
	}
	return packed
}

func UnPackBitRun(packed []byte, bitLen uint64) []byte {
	bits := make([]byte, int(bitLen))
	for idx := uint64(0); idx < bitLen; idx++ {
		if packed[idx/8]&(1<<uint(7-(idx%8))) != 0 {
			bits[idx] = '1'
		} else {
			bits[idx] = '0'
		}
	}
	return bits
}

func PackedBitRunLength(bitLen uint64) uint64 {
	return (bitLen + 7) / 8
}

func HasNonZeroBitPadding(packed []byte, bitLen uint64) bool {
	if len(packed) == 0 || bitLen%8 == 0 {
		return false
	}
	unusedBits := 8 - uint(bitLen%8)
	paddingMask := byte((1 << unusedBits) - 1)
	return packed[len(packed)-1]&paddingMask != 0
}

func GlyphsToBitsPreservingOperands(input string) string {
	var sb strings.Builder
	sb.Grow(len(input) * 2)

	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '-':
			sb.WriteString("00")
		case ',':
			sb.WriteString("01")
		case '.':
			sb.WriteString("10")
		case ';':
			sb.WriteString("11")
		default:
			sb.WriteByte(input[i])
		}
	}

	return sb.String()
}

func BitsAndOperandsToGlyphs(input string) (string, error) {
	var sb strings.Builder
	sb.Grow(len(input))

	for i := 0; i < len(input); {
		if IsBitByte(input[i]) {
			start := i
			for i < len(input) && IsBitByte(input[i]) {
				i++
			}
			run := input[start:i]
			if len(run)%2 != 0 {
				return "", fmt.Errorf("bit run starting at %d has odd length %d", start, len(run))
			}
			for j := 0; j < len(run); j += 2 {
				glyph, ok := GlyphFromBits(run[j : j+2])
				if !ok {
					return "", fmt.Errorf("invalid bit pair %q at %d", run[j:j+2], start+j)
				}
				sb.WriteByte(glyph)
			}
			continue
		}

		end := OperandTokenEnd(input, i)
		sb.WriteString(input[i:end])
		i = end
	}

	return sb.String(), nil
}

func IsBitByte(b byte) bool {
	return b == '0' || b == '1'
}

func GlyphFromBits(bits string) (byte, bool) {
	switch bits {
	case "00":
		return '-', true
	case "01":
		return ',', true
	case "10":
		return '.', true
	case "11":
		return ';', true
	default:
		return 0, false
	}
}

func OperandTokenEndBytes(input []byte, idx int) int {
	if idx >= len(input) {
		return idx
	}

	switch input[idx] {
	case 'h', 'k', 'n', 'g', 'l':
		return idx + 1
	case 'e', 'm', 'd', 'b', 'o', 'x', 'y', 'z':
		if idx+1 >= len(input) {
			return idx + 1
		}
		d1 := input[idx+1]
		if d1 < '1' || d1 > '9' {
			return idx + 1
		}
		if idx+2 < len(input) {
			d2 := input[idx+2]
			if d2 >= '0' && d2 <= '9' {
				combined := int(d1-'0')*10 + int(d2-'0')
				if combined >= 10 && combined <= 31 {
					return idx + 3
				}
			}
		}
		return idx + 2
	case 's':
		if idx+1 < len(input) && input[idx+1] >= '1' && input[idx+1] <= '9' {
			return idx + 2
		}
		return idx + 1
	case 't', 'f':
		if idx+1 < len(input) && IsDirectionByte(input[idx+1]) {
			return idx + 2
		}
		return idx + 1
	case 'r':
		if idx+1 < len(input) && IsAxisByte(input[idx+1]) {
			return idx + 2
		}
		return idx + 1
	case 'i':
		end := idx + 1
		for end < len(input) && input[end] >= '0' && input[end] <= '9' {
			end++
		}
		if end < len(input) && IsVariableByte(input[end]) {
			end++
		}
		if end == idx+1 {
			return idx + 1
		}
		return end
	default:
		return idx + 1
	}
}

func ConsumeDigits(s string, idx int) int {
	for idx < len(s) && s[idx] >= '0' && s[idx] <= '9' {
		idx++
	}
	return idx
}

func IsDirectionByte(b byte) bool {
	switch b {
	case 'u', 'd', 'l', 'r', 'f', 'b':
		return true
	default:
		return false
	}
}

func IsAxisByte(b byte) bool {
	return b == 'x' || b == 'y' || b == 'z'
}

func IsVariableByte(b byte) bool {
	return b >= 'A' && b <= 'Z'
}

func FirstOperandContext(s string) string {
	for idx := 0; idx < len(s); idx++ {
		switch s[idx] {
		case '-', ',', '.', ';':
			continue
		default:
			start := idx - 12
			if start < 0 {
				start = 0
			}
			end := idx + 24
			if end > len(s) {
				end = len(s)
			}
			return fmt.Sprintf("at index %d: %q", idx, s[start:end])
		}
	}
	return "at unknown location"
}

func ContainsOnlyGlyphs(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '-', ',', '.', ';':
			continue
		default:
			return false
		}
	}
	return true
}
