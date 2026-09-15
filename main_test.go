package main

import "testing"

func TestRunReturnsFailureExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing command", args: []string{"laser"}},
		{name: "gui without command", args: []string{"laser", "-gui"}},
		{name: "missing train path", args: []string{"laser", "train"}},
		{name: "missing pack path", args: []string{"laser", "pack"}},
		{name: "invalid train path", args: []string{"laser", "train", "missing-path"}},
		{name: "invalid pack path", args: []string{"laser", "pack", "missing-path"}},
		{name: "invalid compression arguments", args: []string{"laser", "pack", "-c"}},
		{name: "missing unpack path", args: []string{"laser", "unpack"}},
		{name: "unknown command", args: []string{"laser", "unknown"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := run(test.args); got == 0 {
				t.Fatalf("run(%v) returned success for invalid input", test.args)
			}
		})
	}
}
