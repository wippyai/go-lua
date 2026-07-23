package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBranchAlgebraSelectsCanonicalEdgeFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(3)
	root := pathdom.NewPath(symbol.ID(1), "root")
	child := root.Field("kind")
	other := pathdom.NewPath(symbol.ID(2), "other")
	rootFact := factflow.NewBranchRefinement(root, factflow.NewValueConstraint(typevalue.LiteralString(reg, "root")), true, factflow.ValueRefinement{}, false)
	childFact := factflow.NewBranchRefinement(child, factflow.NewValueConstraint(typevalue.LiteralString(reg, "child")), true, factflow.ValueRefinement{}, false)
	trueRelation := factflow.NewBranchPathEquality(root, other, true, false)
	falseRelation := factflow.NewBranchPathInequality(root, other, false, true)
	algebra := NewBranchAlgebra(factflow.NewFacts(factflow.FactsInput{
		BranchRefinements:   map[cfg.Point]factflow.BranchRefinementSet{point: factflow.NewBranchRefinementSet(childFact, rootFact)},
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{point: factflow.NewBranchPathRelationSet(falseRelation, trueRelation)},
	}), point)

	active := algebra.ActiveRefinements(true)
	if len(active) != 2 || !active[0].TargetPathRef().Equal(root) || !active[1].TargetPathRef().Equal(child) {
		t.Fatalf("active refinement order = %#v, want shallow root then child", active)
	}
	relations := algebra.ActivePathRelations(true)
	if len(relations) != 1 || relations[0].Kind() != factflow.BranchPathRelationEqual {
		t.Fatalf("true-edge relations = %#v", relations)
	}
}
