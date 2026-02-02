package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractAssignments_NilConfig(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments:        []flow.UnifiedAssignment{},
		IndexerAssignments: []flow.IndexerAssignment{},
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
		PredicateLinks:     make(map[string]flow.PredicateLink),
	}
	fc := &core.FlowContext{}
	ExtractAssignments(fc, inputs, nil)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments with nil graph, got %d", len(inputs.Assignments))
	}
}

func TestExtractIterSource_NilArgs(t *testing.T) {
	result := resolve.ExtractIteratorSource(nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil iterExprs, got %v", result)
	}
}

func TestExtractIterSource_EmptyExprs(t *testing.T) {
	result := resolve.ExtractIteratorSource([]ast.Expr{}, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for empty iterExprs, got %v", result)
	}
}

func TestExtractIterSource_WithSynth(t *testing.T) {
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return nil, false
	}
	constResolver := func(name string) *flow.ConstValue {
		return nil
	}
	result := resolve.ExtractIteratorSource(
		[]ast.Expr{&ast.IdentExpr{Value: "test"}},
		0,
		synth,
		symResolver,
		constResolver,
		nil,
	)
	_ = result
}

func TestExtractFuncDefAssignments_NilConfig(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	fc := &core.FlowContext{}
	ExtractFuncDefAssignments(fc, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments with nil graph, got %d", len(inputs.Assignments))
	}
}

func TestIterSourceInfo_ZeroValue(t *testing.T) {
	var info resolve.IteratorSourceInfo
	if info.Path.Root != "" {
		t.Errorf("expected empty Path.Root for zero value")
	}
}

func TestExtractIterSource_WithBindings(t *testing.T) {
	bindings := &bind.BindingTable{}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Unknown
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return nil, false
	}
	constResolver := func(name string) *flow.ConstValue {
		return nil
	}
	result := resolve.ExtractIteratorSource(
		[]ast.Expr{&ast.IdentExpr{Value: "iter"}},
		1,
		synth,
		symResolver,
		constResolver,
		bindings,
	)
	_ = result
}
