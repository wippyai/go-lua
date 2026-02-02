package phase

import (
	"testing"

	"github.com/wippyai/go-lua/types/flow"
)

func TestRunSolve_NilInputs(t *testing.T) {
	input := FlowSolveInput{
		Extract: FlowExtractOutput{Inputs: nil},
	}
	output := RunSolve(input)
	if output.Solution != nil {
		t.Error("expected nil solution for nil inputs")
	}
}

func TestRunSolveWithResolver_NilInputs(t *testing.T) {
	output := RunSolveWithResolver(nil, nil)
	if output.Solution != nil {
		t.Error("expected nil solution for nil inputs")
	}
}

func TestRunSolve_EmptyInputs(t *testing.T) {
	inputs := &flow.Inputs{}
	input := FlowSolveInput{
		Extract: FlowExtractOutput{Inputs: inputs},
	}
	output := RunSolve(input)
	if output.Solution == nil {
		t.Error("expected non-nil solution for non-nil inputs")
	}
}

func TestRunSolveWithResolver_EmptyInputs(t *testing.T) {
	inputs := &flow.Inputs{}
	output := RunSolveWithResolver(inputs, nil)
	if output.Solution == nil {
		t.Error("expected non-nil solution for non-nil inputs")
	}
}
