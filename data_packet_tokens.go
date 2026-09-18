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
	"strconv"
)

type PacketToken struct {
	Kind      PacketTokenKind
	Bits      []byte
	BitLen    int
	Raw       []byte
	Op        byte
	Magnitude uint64
	Variable  byte
	Direction byte
	Axis      byte
}

func PacketTokensPackedPayloadSize(tokens []PacketToken) int {
	size := 0
	for _, token := range tokens {
		size += token.PackedPayloadSize()
	}
	return size
}

func (token PacketToken) PackedPayloadSize() int {
	switch token.Kind {
	case PacketTokenBitRun:
		return CompactBitRunEncodedSize(token.BitLength())
	case PacketTokenRawOperandRun:
		return CompactRawOperandRunSize(len(token.Raw))
	default:
		return len(EncodePacketOperandToken(token))
	}
}

func (token PacketToken) BitLength() int {
	if token.BitLen > 0 {
		return token.BitLen
	}
	return len(token.Bits)
}

func PacketTokensFromGlyphOperands(input string) []PacketToken {
	tokens := make([]PacketToken, 0, len(input)/4+1)
	bytesInput := []byte(input)

	for idx := 0; idx < len(bytesInput); {
		if _, ok := GlyphOrBitLength(bytesInput[idx]); ok {
			bits := make([]byte, 0, 16)
			for idx < len(bytesInput) {
				if _, ok := GlyphOrBitLength(bytesInput[idx]); !ok {
					break
				}
				bits = AppendGlyphOrBit(bits, bytesInput[idx])
				idx++
			}
			if len(bits) > 0 {
				tokens = append(tokens, PacketToken{
					Kind:   PacketTokenBitRun,
					Bits:   bits,
					BitLen: len(bits),
				})
			}
			continue
		}

		if token, end, ok := ParseOperandTokenBytes(bytesInput, idx); ok {
			tokens = append(tokens, token)
			idx = end
			continue
		}

		rawStart := idx
		for idx < len(bytesInput) {
			if _, ok := GlyphOrBitLength(bytesInput[idx]); ok {
				break
			}
			if _, _, ok := ParseOperandTokenBytes(bytesInput, idx); ok && idx > rawStart {
				break
			}
			next := OperandTokenEndBytes(bytesInput, idx)
			if next <= idx {
				next = idx + 1
			}
			idx = next
		}
		if idx > rawStart {
			tokens = append(tokens, PacketToken{Kind: PacketTokenRawOperandRun, Raw: bytesInput[rawStart:idx]})
		}
	}

	return tokens
}

func PacketTokensFromBitOperands(input []byte) []PacketToken {
	tokens := make([]PacketToken, 0, len(input)/4+1)

	for idx := 0; idx < len(input); {
		if IsBitByte(input[idx]) {
			start := idx
			for idx < len(input) && IsBitByte(input[idx]) {
				idx++
			}
			tokens = append(tokens, PacketToken{
				Kind:   PacketTokenBitRun,
				Bits:   input[start:idx],
				BitLen: idx - start,
			})
			continue
		}

		if token, end, ok := ParseOperandTokenBytes(input, idx); ok {
			tokens = append(tokens, token)
			idx = end
			continue
		}

		rawStart := idx
		for idx < len(input) && !IsBitByte(input[idx]) {
			if _, _, ok := ParseOperandTokenBytes(input, idx); ok && idx > rawStart {
				break
			}
			next := OperandTokenEndBytes(input, idx)
			if next <= idx {
				next = idx + 1
			}
			idx = next
		}
		if idx > rawStart {
			tokens = append(tokens, PacketToken{Kind: PacketTokenRawOperandRun, Raw: input[rawStart:idx]})
		}
	}

	return tokens
}

func GlyphOrBitLength(b byte) (int, bool) {
	switch b {
	case '-', ',', '.', ';':
		return 2, true
	case '0', '1':
		return 1, true
	default:
		return 0, false
	}
}

func AppendGlyphOrBit(bits []byte, b byte) []byte {
	switch b {
	case '-':
		return append(bits, '0', '0')
	case ',':
		return append(bits, '0', '1')
	case '.':
		return append(bits, '1', '0')
	case ';':
		return append(bits, '1', '1')
	case '0', '1':
		return append(bits, b)
	default:
		return bits
	}
}

func ParseOperandTokenBytes(input []byte, idx int) (PacketToken, int, bool) {
	if idx >= len(input) {
		return PacketToken{}, idx, false
	}

	switch input[idx] {
	case 'h', 'k', 'n', 'g', 'l':
		return PacketToken{Kind: PacketTokenFixedOperand, Op: input[idx]}, idx + 1, true
	case 'e', 'm', 'd', 'b', 'o', 'x', 'y', 'z':
		end := OperandTokenEndBytes(input, idx)
		if end != idx+2 || idx+1 >= len(input) || input[idx+1] < '1' || input[idx+1] > '9' {
			return PacketToken{}, idx, false
		}
		return PacketToken{
			Kind:      PacketTokenMagnitudeOperand,
			Op:        input[idx],
			Magnitude: uint64(input[idx+1] - '0'),
		}, end, true
	case 't', 'f':
		if idx+1 >= len(input) || !IsDirectionByte(input[idx+1]) {
			return PacketToken{}, idx, false
		}
		return PacketToken{Kind: PacketTokenDirectionalOperand, Op: input[idx], Direction: input[idx+1]}, idx + 2, true
	case 'r':
		if idx+1 >= len(input) || !IsAxisByte(input[idx+1]) {
			return PacketToken{}, idx, false
		}
		return PacketToken{Kind: PacketTokenAxisOperand, Op: input[idx], Axis: input[idx+1]}, idx + 2, true
	case 'i':
		end := idx + 1
		for end < len(input) && input[end] >= '0' && input[end] <= '9' {
			end++
		}
		if end == idx+1 || end >= len(input) || !IsVariableByte(input[end]) {
			return PacketToken{}, idx, false
		}
		lengthVal, err := strconv.ParseUint(string(input[idx+1:end]), 10, 64)
		if err != nil || lengthVal == 0 {
			return PacketToken{}, idx, false
		}
		return PacketToken{
			Kind:      PacketTokenIndexOperand,
			Magnitude: lengthVal,
			Variable:  input[end] - 'A',
		}, end + 1, true
	default:
		if IsVariableByte(input[idx]) {
			return PacketToken{Kind: PacketTokenVariableRef, Variable: input[idx] - 'A'}, idx + 1, true
		}
		return PacketToken{}, idx, false
	}
}

func EncodePacketOperandToken(token PacketToken) []byte {
	switch token.Kind {
	case PacketTokenFixedOperand:
		switch token.Op {
		case 'h':
			return []byte{compactOperandFixedBase}
		case 'k':
			return []byte{compactOperandFixedBase + 1}
		case 'n':
			return []byte{compactOperandFixedBase + 2}
		case 'g':
			return []byte{compactOperandFixedBase + 3}
		case 'l':
			return []byte{compactOperandFixedBase + 4}
		}
	case PacketTokenMagnitudeOperand:
		if token.Magnitude < 1 || token.Magnitude > 9 {
			return nil
		}
		if magOpIdx := CompactMagnitudeOperandIndex(token.Op); magOpIdx >= 0 {
			return []byte{compactOperandMagBase + byte(magOpIdx*9+int(token.Magnitude-1))}
		}
	case PacketTokenDirectionalOperand:
		if dirIdx := DirectionIndex(token.Direction); dirIdx >= 0 {
			base := 0
			if token.Op == 'f' {
				base = len(directionClasses)
			} else if token.Op != 't' {
				return nil
			}
			return []byte{compactOperandDirBase + byte(base+dirIdx)}
		}
	case PacketTokenAxisOperand:
		if token.Op == 'r' {
			if axisIdx := AxisIndex(token.Axis); axisIdx >= 0 {
				return []byte{compactOperandRotBase + byte(axisIdx)}
			}
		}
	case PacketTokenIndexOperand:
		if token.Magnitude == 0 || token.Variable >= variableTokenCount {
			return nil
		}
		if token.Magnitude <= 8 {
			param := byte((token.Magnitude-1)<<5) | token.Variable
			return []byte{compactOperandIndex, param}
		}
		if token.Magnitude <= 255 {
			return []byte{compactOperandIndexLong, byte(token.Magnitude), token.Variable}
		}
		var lengthBuf [binary.MaxVarintLen64]byte
		n := binary.PutUvarint(lengthBuf[:], token.Magnitude)
		encoded := make([]byte, 1+n+1)
		encoded[0] = compactOperandIndexVar
		copy(encoded[1:], lengthBuf[:n])
		encoded[len(encoded)-1] = token.Variable
		return encoded
	case PacketTokenVariableRef:
		if token.Variable < variableTokenCount {
			return []byte{compactOperandVarBase + token.Variable}
		}
	}
	return nil
}

func WritePacketToken(out *bytes.Buffer, token PacketToken) bool {
	switch token.Kind {
	case PacketTokenBitRun:
		WriteCompactBitRun(out, token.Bits)
		return true
	case PacketTokenRawOperandRun:
		WriteCompactRawOperandRun(out, token.Raw)
		return true
	default:
		encoded := EncodePacketOperandToken(token)
		if len(encoded) == 0 {
			return false
		}
		out.Write(encoded)
		return true
	}
}

func ReadPacketOperandToken(reader *bytes.Reader, token byte) (PacketToken, error) {
	switch {
	case token >= compactOperandFixedBase && token < compactOperandMagBase:
		return PacketToken{
			Kind: PacketTokenFixedOperand,
			Op:   []byte{'h', 'k', 'n', 'g', 'l'}[token-compactOperandFixedBase],
		}, nil
	case token >= compactOperandMagBase && token < compactOperandDirBase:
		offset := int(token - compactOperandMagBase)
		ops := []byte{'e', 'm', 'd', 'b', 'o', 'x', 'y', 'z'}
		return PacketToken{
			Kind:      PacketTokenMagnitudeOperand,
			Op:        ops[offset/9],
			Magnitude: uint64(offset%9 + 1),
		}, nil
	case token >= compactOperandDirBase && token < compactOperandRotBase:
		offset := int(token - compactOperandDirBase)
		op := byte('t')
		if offset >= len(directionClasses) {
			op = 'f'
			offset -= len(directionClasses)
		}
		return PacketToken{
			Kind:      PacketTokenDirectionalOperand,
			Op:        op,
			Direction: directionClasses[offset][0],
		}, nil
	case token >= compactOperandRotBase && token < compactOperandIndex:
		return PacketToken{
			Kind: PacketTokenAxisOperand,
			Op:   'r',
			Axis: axisClasses[token-compactOperandRotBase][0],
		}, nil
	case token == compactOperandIndex:
		param, err := reader.ReadByte()
		if err != nil {
			return PacketToken{}, fmt.Errorf("failed to read compact index operand: %w", err)
		}
		variableIdx := param & 0x1f
		if variableIdx >= variableTokenCount {
			return PacketToken{}, fmt.Errorf("compact index operand has invalid variable index %d", variableIdx)
		}
		return PacketToken{
			Kind:      PacketTokenIndexOperand,
			Magnitude: uint64(param>>5) + 1,
			Variable:  variableIdx,
		}, nil
	case token == compactOperandIndexLong:
		lengthByte, err := reader.ReadByte()
		if err != nil {
			return PacketToken{}, fmt.Errorf("failed to read compact long index length: %w", err)
		}
		variableByte, err := reader.ReadByte()
		if err != nil {
			return PacketToken{}, fmt.Errorf("failed to read compact long index variable: %w", err)
		}
		if lengthByte == 0 {
			return PacketToken{}, fmt.Errorf("compact long index operand has zero length")
		}
		if variableByte >= variableTokenCount {
			return PacketToken{}, fmt.Errorf("compact long index operand has invalid variable index %d", variableByte)
		}
		return PacketToken{
			Kind:      PacketTokenIndexOperand,
			Magnitude: uint64(lengthByte),
			Variable:  variableByte,
		}, nil
	case token == compactOperandIndexVar:
		lengthVal, err := binary.ReadUvarint(reader)
		if err != nil {
			return PacketToken{}, fmt.Errorf("failed to read compact varint index length: %w", err)
		}
		variableByte, err := reader.ReadByte()
		if err != nil {
			return PacketToken{}, fmt.Errorf("failed to read compact varint index variable: %w", err)
		}
		if lengthVal == 0 {
			return PacketToken{}, fmt.Errorf("compact varint index operand has zero length")
		}
		if variableByte >= variableTokenCount {
			return PacketToken{}, fmt.Errorf("compact varint index operand has invalid variable index %d", variableByte)
		}
		return PacketToken{
			Kind:      PacketTokenIndexOperand,
			Magnitude: lengthVal,
			Variable:  variableByte,
		}, nil
	case token == compactOperandRawRun:
		length, err := binary.ReadUvarint(reader)
		if err != nil {
			return PacketToken{}, fmt.Errorf("failed to read compact raw operand length: %w", err)
		}
		if length > uint64(reader.Len()) {
			return PacketToken{}, fmt.Errorf("compact raw operand length %d exceeds remaining payload %d", length, reader.Len())
		}
		payload := make([]byte, int(length))
		if _, err := reader.Read(payload); err != nil {
			return PacketToken{}, fmt.Errorf("failed to read compact raw operand run: %w", err)
		}
		return PacketToken{Kind: PacketTokenRawOperandRun, Raw: payload}, nil
	case token >= compactOperandVarBase && token < compactOperandVarBase+byte(variableTokenCount):
		return PacketToken{Kind: PacketTokenVariableRef, Variable: token - compactOperandVarBase}, nil
	default:
		return PacketToken{}, fmt.Errorf("binary bitops input has unknown compact operand token 0x%02x", token)
	}
}

func WritePacketTokenText(out *bytes.Buffer, token PacketToken) {
	switch token.Kind {
	case PacketTokenFixedOperand:
		out.WriteByte(token.Op)
	case PacketTokenMagnitudeOperand:
		out.WriteByte(token.Op)
		out.WriteString(strconv.FormatUint(token.Magnitude, 10))
	case PacketTokenDirectionalOperand:
		out.WriteByte(token.Op)
		out.WriteByte(token.Direction)
	case PacketTokenAxisOperand:
		out.WriteByte(token.Op)
		out.WriteByte(token.Axis)
	case PacketTokenIndexOperand:
		out.WriteByte('i')
		out.WriteString(strconv.FormatUint(token.Magnitude, 10))
		out.WriteByte(byte('A') + token.Variable)
	case PacketTokenVariableRef:
		out.WriteByte(byte('A') + token.Variable)
	case PacketTokenRawOperandRun:
		out.Write(token.Raw)
	case PacketTokenBitRun:
		out.Write(token.Bits)
	}
}

func PackedInstructionSize(instructions string) int {
	return len(bitOpsBinaryMagic) + PacketTokensPackedPayloadSize(PacketTokensFromGlyphOperands(instructions))
}

func UVarIntLength(value uint64) int {
	length := 1
	for value >= 0x80 {
		value >>= 7
		length++
	}
	return length
}

func CompactBitRunEncodedSize(bitLen int) int {
	size := 0
	for bitLen > 0 {
		chunk := bitLen
		if chunk > compactBitRunMax {
			chunk = compactBitRunMax
		}
		size += 1 + int(PackedBitRunLength(uint64(chunk)))
		bitLen -= chunk
	}
	return size
}

func CompactRawOperandRunSize(length int) int {
	if length <= 0 {
		return 0
	}
	return 1 + UVarIntLength(uint64(length)) + length
}
