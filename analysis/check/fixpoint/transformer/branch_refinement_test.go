package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBranchRefinementTermMatchesConcreteRootConstraint(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(3)
	target := pathdom.NewPath(symbol.ID(7), "value")
	constraint := typevalue.LiteralString(reg, "ready")
	refinement := factflow.NewValueConstraint(constraint)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	source, _ := factflow.NewPathValueSource("value", 0, 0, 0, shape)
	condition, _ := factflow.NewBranchCondition(source, true)
	branch := factapply.NewBranchAlgebra(factflow.NewFacts(factflow.FactsInput{
		BranchConditionSources: map[cfg.Point]factflow.BranchCondition{point: condition},
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{point: factflow.NewBranchRefinementSet(
			factflow.NewBranchRefinement(target, refinement, true, factflow.ValueRefinement{}, false),
		)},
	}), point)

	base := product.Join(reg, typevalue.LiteralString(reg, "ready"), typevalue.LiteralString(reg, "done"))
	arena := NewArena(reg)
	root := arena.Root(Root{Kind: RootParam, Index: 0})
	updates, err := LowerBranchRefinements(arena, branch, true, func(got pathdom.Path) (ValueTerm, bool) {
		return root, got.Equal(target)
	})
	if err != nil || len(updates) != 1 {
		t.Fatalf("symbolic refinements = %#v, %v", updates, err)
	}
	cursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{base}, nil)
	symbolic, ok := arena.evalValue(updates[0].Value(), cursor, SpecializationContext{})
	if !ok {
		t.Fatal("symbolic refinement did not evaluate")
	}

	input := state.State{}.WriteValue(reg, statekey.SymbolValue(target.Symbol), base)
	concrete, reachable := factapply.ApplyConcreteGuardRefinement(factapply.ConcreteGuardRefinementRequest{
		Registry: reg, Point: point, Input: input, Output: input, TargetPath: target, Refinement: refinement,
	})
	if !reachable {
		t.Fatal("concrete refinement unexpectedly contradicted")
	}
	want := concrete.ReadValue(reg, statekey.SymbolValue(target.Symbol))
	if !product.Equal(reg, symbolic, want) {
		t.Fatalf("symbolic/concrete refinement differs\n symbolic=%#v\n concrete=%#v", symbolic, want)
	}
}

func TestBranchRefinementTermRebasesTransactionally(t *testing.T) {
	reg := standard.Registry()
	callee := NewArena(reg)
	root := callee.Root(Root{Kind: RootParam, Index: 0})
	refined, ok := callee.RefineValue(root, factflow.NewValueConstraint(typevalue.LiteralString(reg, "ready")))
	if !ok {
		t.Fatal("positive refinement rejected")
	}
	caller := NewArena(reg)
	callerRoot := caller.Root(Root{Kind: RootParam, Index: 0})
	bindings, err := NewTermRootBindings(Shape{Params: 1}, Shape{Params: 1}, []ValueTerm{callerRoot}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{refined}})
	if err != nil || len(got.Values) != 1 {
		t.Fatalf("rebase = %#v, %v", got, err)
	}
	cursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{product.Top()}, nil)
	value, exact := caller.evalValue(got.Values[0], cursor, SpecializationContext{})
	if !exact || !product.Equal(reg, value, factapply.RefineProductValueConstraint(reg, product.Top(), typevalue.LiteralString(reg, "ready"))) {
		t.Fatalf("rebased refinement = %#v/%v", value, exact)
	}
}
