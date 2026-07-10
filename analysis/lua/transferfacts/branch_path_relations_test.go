package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func TestCheckPathRelationsDirectAndImpliedLaws(t *testing.T) {
	left := path.NewPath(1, "left")
	right := path.NewPath(2, "right")

	direct := checkPathRelations(branchcond.Check{Kind: branchcond.CheckPathEqual, Path: left, OtherPath: right}, true, true)
	assertBranchPathRelation(t, direct, factflow.BranchPathRelationEqual, left, right, true)
	assertBranchPathRelation(t, direct, factflow.BranchPathRelationNotEqual, left, right, false)

	implied := checkPathRelationsForImplication(branchcond.ImpliedCheck{
		Check:    branchcond.Check{Kind: branchcond.CheckTypeEqual, Path: left, OtherPath: right},
		Edge:     false,
		Polarity: false,
	})
	assertBranchPathRelation(t, implied, factflow.BranchPathRelationTypeUnmatch, left, right, false)
	if len(implied) != 1 {
		t.Fatalf("implied relation should publish only one edge fact, got %#v", implied)
	}
}

func assertBranchPathRelation(
	t *testing.T,
	relations []factflow.BranchPathRelation,
	kind factflow.BranchPathRelationKind,
	left path.Path,
	right path.Path,
	edge bool,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.Kind() == kind &&
			relation.LeftPath().Equal(left) &&
			relation.RightPath().Equal(right) &&
			relation.ActiveOnEdge(edge) {
			return
		}
	}
	t.Fatalf("missing relation kind=%v edge=%v in %#v", kind, edge, relations)
}
