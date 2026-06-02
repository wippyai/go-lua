package effects

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferLocalReturnFlowRow_ParameterFieldIntoReturnField(t *testing.T) {
	stmts, err := parse.ParseString(`
local error_message = info.message or "fallback"
return {
  error_message = error_message,
  metadata = { status_code = info.status_code },
}
`, "return_flow.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"info"}},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, "mapper.map_error_response")
	if graph == nil {
		t.Fatal("expected graph")
	}
	result := &api.FuncResult{Graph: graph}
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		result.Evidence.Assignments = append(result.Evidence.Assignments, api.AssignmentEvidence{Point: p, Info: info})
	})

	row := inferLocalReturnFlowRow(result)
	message := findFlowInto(row, 0, effect.FieldPath("message"), 0, effect.FieldPath("error_message"))
	if message == nil {
		t.Fatalf("missing info.message -> return.error_message flow in %#v", row.Labels)
	}
	if !typ.TypeEquals(message.Remainder, typ.LiteralString("fallback")) {
		t.Fatalf("message remainder = %v, want fallback literal", message.Remainder)
	}

	status := findFlowInto(row, 0, effect.FieldPath("status_code"), 0, effect.FieldPath("metadata", "status_code"))
	if status == nil {
		t.Fatalf("missing info.status_code -> return.metadata.status_code flow in %#v", row.Labels)
	}
	if status.Remainder != nil {
		t.Fatalf("status remainder = %v, want nil", status.Remainder)
	}
}

func TestInferLocalReturnFlowRow_DirectParameterFieldIntoReturnField(t *testing.T) {
	stmts, err := parse.ParseString(`
return {
  error_message = info.message or "fallback",
  metadata = { status_code = info.status_code },
}
`, "return_flow.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"info"}},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, "mapper.map_error_response")
	if graph == nil {
		t.Fatal("expected graph")
	}

	row := inferLocalReturnFlowRow(&api.FuncResult{Graph: graph})
	message := findFlowInto(row, 0, effect.FieldPath("message"), 0, effect.FieldPath("error_message"))
	if message == nil {
		t.Fatalf("missing info.message -> return.error_message flow in %#v", row.Labels)
	}
	if !typ.TypeEquals(message.Remainder, typ.LiteralString("fallback")) {
		t.Fatalf("message remainder = %v, want fallback literal", message.Remainder)
	}

	status := findFlowInto(row, 0, effect.FieldPath("status_code"), 0, effect.FieldPath("metadata", "status_code"))
	if status == nil {
		t.Fatalf("missing info.status_code -> return.metadata.status_code flow in %#v", row.Labels)
	}
	if status.Remainder != nil {
		t.Fatalf("status remainder = %v, want nil", status.Remainder)
	}
}

func TestInferLocalReturnFlowRow_BracketStringKeyUsesStructuralSegment(t *testing.T) {
	stmts, err := parse.ParseString(`
return {
  ["error_message"] = info.message,
}
`, "return_flow_bracket.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"info"}},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, "mapper.map_error_response")
	if graph == nil {
		t.Fatal("expected graph")
	}

	row := inferLocalReturnFlowRow(&api.FuncResult{Graph: graph})
	message := findFlowInto(row, 0, effect.FieldPath("message"), 0, effect.PathSuffix{
		{Kind: constraint.SegmentIndexString, Name: "error_message"},
	})
	if message == nil {
		t.Fatalf("missing bracket string return flow in %#v", row.Labels)
	}
}

func TestInferLocalReturnFlowRow_StaticIndexesDoNotCollapseToDottedPath(t *testing.T) {
	stmts, err := parse.ParseString(`
return {
  ["error.message"] = info["source.value"],
  [1] = info[2],
}
`, "return_flow_static_indexes.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"info"}},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, "mapper.static_indexes")
	if graph == nil {
		t.Fatal("expected graph")
	}

	row := inferLocalReturnFlowRow(&api.FuncResult{Graph: graph})
	stringIndex := findFlowInto(row, 0, effect.PathSuffix{
		{Kind: constraint.SegmentIndexString, Name: "source.value"},
	}, 0, effect.PathSuffix{
		{Kind: constraint.SegmentIndexString, Name: "error.message"},
	})
	if stringIndex == nil {
		t.Fatalf("missing structural string-index flow in %#v", row.Labels)
	}

	intIndex := findFlowInto(row, 0, effect.PathSuffix{
		{Kind: constraint.SegmentIndexInt, Index: 2},
	}, 0, effect.PathSuffix{
		{Kind: constraint.SegmentIndexInt, Index: 1},
	})
	if intIndex == nil {
		t.Fatalf("missing structural int-index flow in %#v", row.Labels)
	}
}

func findFlowInto(row effect.Row, paramIndex int, sourcePath effect.PathSuffix, returnIndex int, targetPath effect.PathSuffix) *effect.FlowInto {
	for _, label := range row.Labels {
		flow, ok := label.(effect.FlowInto)
		if !ok {
			continue
		}
		if flow.ParamIndex == paramIndex &&
			flow.SourcePath.Equal(sourcePath) &&
			flow.ReturnIndex == returnIndex &&
			flow.TargetPath.Equal(targetPath) {
			return &flow
		}
	}
	return nil
}
