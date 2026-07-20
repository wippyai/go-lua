package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

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
