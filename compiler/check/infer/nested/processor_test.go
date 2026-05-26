package nestedinfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProcessNestedFunctions_NilResult(t *testing.T) {
	p := New(Config{})
	p.ProcessNestedFunctions(nil, nil)
}

func TestProcessNestedFunctions_NilScopes(t *testing.T) {
	p := New(Config{})
	p.ProcessNestedFunctions(nil, &api.FuncAnalysisView{})
}

func TestNestedAnalysisContext_UsesCallEvidenceExpectedArgsWithoutNarrowSynth(t *testing.T) {
	stmts, err := parse.ParseString(`register(function(value) end)`, "nested_callback.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	caller := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	bindings := bind.Bind(caller, []string{"register"})
	graph := cfg.BuildWithBindings(caller, bindings)

	var callPoint cfg.Point
	var callInfo *cfg.CallInfo
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if callInfo == nil {
			callPoint = p
			callInfo = info
		}
	})
	if callInfo == nil || len(callInfo.Args) != 1 {
		t.Fatalf("expected one callback call, got %+v", callInfo)
	}
	callback, ok := callInfo.Args[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("arg type = %T, want function literal", callInfo.Args[0])
	}

	expected := typ.Func().Param("value", typ.String).Build()
	parent := &api.FuncAnalysisView{
		Graph: graph,
		Evidence: api.FlowEvidence{
			Calls: []api.CallEvidence{{
				Point:        callPoint,
				Info:         callInfo,
				ExpectedArgs: []typ.Type{expected},
			}},
		},
	}

	ctx := New(Config{}).nestedAnalysisContext(callback, parent)
	if ctx.ExpectedFunction == nil {
		t.Fatal("expected callback analysis context without NarrowSynth")
	}
	if len(ctx.ExpectedFunction.Params) != 1 || !typ.TypeEquals(ctx.ExpectedFunction.Params[0].Type, typ.String) {
		t.Fatalf("expected function = %v, want string parameter", ctx.ExpectedFunction)
	}
}

func TestNestedAnalysisContext_InheritsAmbientOverlay(t *testing.T) {
	stmts, err := parse.ParseString(`register(function(value) end)`, "nested_callback.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	caller := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	bindings := bind.Bind(caller, []string{"register"})
	graph := cfg.BuildWithBindings(caller, bindings)

	var callPoint cfg.Point
	var callInfo *cfg.CallInfo
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if callInfo == nil {
			callPoint = p
			callInfo = info
		}
	})
	if callInfo == nil || len(callInfo.Args) != 1 {
		t.Fatalf("expected one callback call, got %+v", callInfo)
	}
	callback, ok := callInfo.Args[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("arg type = %T, want function literal", callInfo.Args[0])
	}

	ambient := typ.Func().Build()
	parent := &api.FuncAnalysisView{
		Graph: graph,
		AnalysisContext: api.AnalysisContext{
			GlobalOverlay:    map[string]product.AbstractValue{"up": product.FromType(ambient)},
			ExpectedFunction: typ.Func().Returns(typ.Nil).Build(),
		},
		Evidence: api.FlowEvidence{
			Calls: []api.CallEvidence{{
				Point: callPoint,
				Info:  callInfo,
			}},
		},
	}

	ctx := New(Config{}).nestedAnalysisContext(callback, parent)
	if !typ.TypeEquals(ctx.GlobalOverlay["up"].ProjectValue(), ambient) {
		t.Fatalf("expected ambient overlay to propagate to nested callback, got %v", ctx.GlobalOverlay)
	}
	if ctx.ExpectedFunction != nil {
		t.Fatalf("expected call-edge expected function not to propagate lexically, got %v", ctx.ExpectedFunction)
	}
}

func TestNestedAnalysisContext_UsesParentLiteralSignature(t *testing.T) {
	stmts, err := parse.ParseString(`return function(value) end`, "nested_return.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	caller := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	bindings := bind.Bind(caller, nil)
	graph := cfg.BuildWithBindings(caller, bindings)

	var returned *ast.FunctionExpr
	for _, nf := range graph.NestedFunctions() {
		if nf.Func != nil {
			returned = nf.Func
			break
		}
	}
	if returned == nil {
		t.Fatal("expected returned function literal")
	}

	expected := typ.Func().Param("value", typ.String).Build()
	parent := &api.FuncAnalysisView{
		Graph:             graph,
		LiteralSignatures: map[*ast.FunctionExpr]*typ.Function{returned: expected},
	}

	ctx := New(Config{}).nestedAnalysisContext(returned, parent)
	if ctx.ExpectedFunction == nil {
		t.Fatal("expected returned literal analysis context")
	}
	if len(ctx.ExpectedFunction.Params) != 1 || !typ.TypeEquals(ctx.ExpectedFunction.Params[0].Type, typ.String) {
		t.Fatalf("expected function = %v, want string parameter", ctx.ExpectedFunction)
	}
}
