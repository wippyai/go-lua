package check

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFuncResultEqual_UsesSemanticStateNotPointerIdentity(t *testing.T) {
	base := scope.New()
	left := &api.FuncResult{
		BaseScope: base,
		Scopes:    map[cfg.Point]*scope.State{1: base},
		FlowInputs: &flow.Inputs{
			DeclaredTypes: map[cfg.SymbolID]typ.Type{1: typ.String},
		},
	}
	right := &api.FuncResult{
		BaseScope: base,
		Scopes:    map[cfg.Point]*scope.State{1: base},
		FlowInputs: &flow.Inputs{
			DeclaredTypes: map[cfg.SymbolID]typ.Type{1: typ.String},
		},
	}

	if left == right {
		t.Fatal("test requires distinct result allocations")
	}
	if !funcResultEqual(left, right) {
		t.Fatal("semantically equal function results should compare equal")
	}
}

func TestFuncResultEqual_IgnoresTransientFlowSolutionState(t *testing.T) {
	base := scope.New()
	inputs := &flow.Inputs{
		DeclaredTypes: map[cfg.SymbolID]typ.Type{1: typ.String},
	}
	left := &api.FuncResult{
		BaseScope:    base,
		Scopes:       map[cfg.Point]*scope.State{1: base},
		FlowInputs:   inputs,
		FlowSolution: &flow.Solution{},
	}
	right := &api.FuncResult{
		BaseScope:  base,
		Scopes:     map[cfg.Point]*scope.State{1: base},
		FlowInputs: inputs,
	}

	if !funcResultEqual(left, right) {
		t.Fatal("query dependency equality must not deep-compare transient flow solutions")
	}
}

func TestFuncResultEqual_IgnoresTransientScopeMaps(t *testing.T) {
	leftScope := scope.New().WithType("LocalOnly", typ.String)
	rightScope := scope.New().WithType("LocalOnly", typ.Number)
	inputs := &flow.Inputs{
		DeclaredTypes: map[cfg.SymbolID]typ.Type{1: typ.String},
	}
	left := &api.FuncResult{
		BaseScope:  leftScope,
		Scopes:     map[cfg.Point]*scope.State{1: leftScope},
		FlowInputs: inputs,
	}
	right := &api.FuncResult{
		BaseScope:  rightScope,
		Scopes:     map[cfg.Point]*scope.State{1: rightScope},
		FlowInputs: inputs,
	}

	if !funcResultEqual(left, right) {
		t.Fatal("scope maps are transient query values; dependency equality should use published products")
	}

	right.FlowInputs = &flow.Inputs{
		DeclaredTypes: map[cfg.SymbolID]typ.Type{1: typ.Number},
	}
	if !funcResultEqual(left, right) {
		t.Fatal("flow inputs are local interpreter problem state; dependency equality should use published products")
	}
}

func TestFuncResultEqual_IgnoresRecursiveFlowInputs(t *testing.T) {
	leftRecursive := typ.NewRecursive("Builder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("add", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})
	rightRecursive := typ.NewRecursive("Builder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("add", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})
	left := &api.FuncResult{
		FlowInputs: &flow.Inputs{
			DeclaredTypes: map[cfg.SymbolID]typ.Type{1: leftRecursive},
		},
	}
	right := &api.FuncResult{
		FlowInputs: &flow.Inputs{
			DeclaredTypes: map[cfg.SymbolID]typ.Type{1: rightRecursive},
		},
	}

	if !funcResultEqual(left, right) {
		t.Fatal("recursive flow inputs must not participate in query dependency equality")
	}
}

func TestFuncResultEqual_UsesRecursiveSafeFactEqualityForLiteralSigs(t *testing.T) {
	fn := &ast.FunctionExpr{}
	leftRecursive := typ.NewRecursive("Builder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("add", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})
	rightRecursive := typ.NewRecursive("Builder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("add", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})
	left := &api.FuncResult{
		LiteralSignatures: api.LiteralSigs{
			fn: typ.Func().Param("self", leftRecursive).Returns(leftRecursive).Build(),
		},
	}
	right := &api.FuncResult{
		LiteralSignatures: api.LiteralSigs{
			fn: typ.Func().Param("self", rightRecursive).Returns(rightRecursive).Build(),
		},
	}

	if !funcResultEqual(left, right) {
		t.Fatal("recursive literal signatures should compare through value-domain fact equality")
	}
}

func TestFuncResultQueryCycleConvergesOnSemanticEquality(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	base := scope.New()
	calls := 0

	var q *db.Query[string, *api.FuncResult]
	q = db.NewQuery("test.func-result", func(ctx *db.QueryContext, key string) *api.FuncResult {
		calls++
		if calls < 3 {
			_ = q.Get(ctx, key)
		}
		return &api.FuncResult{
			BaseScope: base,
			Scopes:    map[cfg.Point]*scope.State{1: base},
			FlowInputs: &flow.Inputs{
				DeclaredTypes: map[cfg.SymbolID]typ.Type{1: typ.String},
			},
		}
	}, funcResultEqual)

	if got := q.Get(ctx, "root"); got == nil {
		t.Fatal("expected converged function result")
	}
	if calls != 2 {
		t.Fatalf("query cycle should converge after one semantic repeat, calls=%d", calls)
	}
}
