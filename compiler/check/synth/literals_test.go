package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
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

func TestExpectedArgsForDirectCallbackLiteral_SkipsCallsWithoutFunctionLiteralArgs(t *testing.T) {
	if callHasDirectFunctionLiteralArg(&cfg.CallInfo{
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.StringExpr{Value: "x"}},
	}) {
		t.Fatal("non-callback call should not request expected callback arguments")
	}
	if !callHasDirectFunctionLiteralArg(&cfg.CallInfo{
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.FunctionExpr{}},
	}) {
		t.Fatal("direct function literal argument should request expected callback arguments")
	}
}
