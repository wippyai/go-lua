package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/api"
)

func TestFunctionLiteralTypes_NilGraph(t *testing.T) {
	result := FunctionLiteralTypes(nil, api.FlowEvidence{}, nil)
	if result != nil {
		t.Fatal("expected nil for nil graph")
	}
}

func TestFunctionLiteralTypes_NilSynth(t *testing.T) {
	result := FunctionLiteralTypes(nil, api.FlowEvidence{}, nil)
	if result != nil {
		t.Fatal("expected nil for nil synth")
	}
}

func TestFunctionLiteralSignatures_NilGraph(t *testing.T) {
	result := FunctionLiteralSignatures(nil, api.FlowEvidence{}, nil, nil)
	if result != nil {
		t.Fatal("expected nil for nil graph")
	}
}

func TestFunctionLiteralSignatures_NilEngine(t *testing.T) {
	result := FunctionLiteralSignatures(nil, api.FlowEvidence{}, nil, nil)
	if result != nil {
		t.Fatal("expected nil for nil engine")
	}
}
