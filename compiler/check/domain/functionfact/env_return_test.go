package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractEnvironmentReturns_FromSolvedReturnCall(t *testing.T) {
	stmts, err := parse.ParseString(`return generate._mapper.map_error_response("bad")`, "env_return.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	graph := cfg.Build(fn, "generate")
	if graph == nil || graph.Bindings() == nil {
		t.Fatal("expected graph with bindings")
	}
	const fnSym cfg.SymbolID = 9000
	graph.Bindings().SetName(fnSym, "generate.handler")

	result := &api.FuncResult{Graph: graph}
	specs := ExtractEnvironmentReturns(result, fnSym, observation.FromFuncResult(result, nil))
	if len(specs) != 1 {
		t.Fatalf("ExtractEnvironmentReturns() returned %d specs, want 1: %#v", len(specs), specs)
	}
	spec := specs[0]
	if spec.ReturnIndex != 0 || spec.ResultIndex != 0 {
		t.Fatalf("return/result index = %d/%d, want 0/0", spec.ReturnIndex, spec.ResultIndex)
	}
	if len(spec.Path) != 2 ||
		spec.Path[0].Kind != constraint.SegmentField ||
		spec.Path[0].Name != "_mapper" ||
		spec.Path[1].Kind != constraint.SegmentField ||
		spec.Path[1].Name != "map_error_response" {
		t.Fatalf("path = %#v, want _mapper.map_error_response", spec.Path)
	}
	if len(spec.Args) != 1 || !typ.TypeEquals(spec.Args[0], typ.LiteralString("bad")) {
		t.Fatalf("args = %v, want [\"bad\"]", spec.Args)
	}
}
