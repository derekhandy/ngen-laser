package main

import "testing"

func TestAllOperandSpellingsPopulatesEchoCandidates(t *testing.T) {
	operands := AllOperandSpellingsForLength(&Rollout{}, 20, 10)
	seen := make(map[string]bool)
	var echoOperands []string
	for _, operand := range operands {
		if operand == "" {
			t.Fatal("operand list contains an empty candidate")
		}
		if len(operand) > 0 && operand[0] == 'e' {
			echoOperands = append(echoOperands, operand)
			if seen[operand] {
				t.Fatalf("duplicate echo candidate %q", operand)
			}
			seen[operand] = true
		}
	}
	want := []string{"e1", "e2", "e3", "e4", "e5", "e6", "e7", "e8", "e9"}
	if len(echoOperands) != len(want) {
		t.Fatalf("echo candidates = %v, want %v", echoOperands, want)
	}
	for i := range want {
		if echoOperands[i] != want[i] {
			t.Fatalf("echo candidates = %v, want %v", echoOperands, want)
		}
	}
}

func TestComputeInstructionsRunsExhaustiveWorkers(t *testing.T) {
	env := NewCompressionEnvironment(";;;", &Rollout{})
	if _, _, err := ComputeInstructions(env, nil, []int{0, 0}, 1, 1); err != nil {
		t.Fatal(err)
	}
}
