package factflow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestExpressionConditionBranchRefinementsForValue(t *testing.T) {
	shared := path.NewPath(symbol.ID(1), "value")
	falseOnly := path.NewPath(symbol.ID(2), "err")
	whenTrue := valueRefinementWithPresence(presence.Present())
	whenFalse := valueRefinementWithPresence(presence.Absent())
	errAbsent := valueRefinementWithPresence(presence.Absent())

	condition := NewExpressionCondition(
		[]PostconditionRefinement{NewPostconditionRefinement(shared, whenTrue)},
		[]PostconditionRefinement{
			NewPostconditionRefinement(shared, whenFalse),
			NewPostconditionRefinement(falseOnly, errAbsent),
		},
		nil,
		nil,
	)

	direct := condition.BranchRefinementsForValue(true)
	if len(direct) != 2 {
		t.Fatalf("direct refinements = %d, want 2: %#v", len(direct), direct)
	}
	assertPathEqual(t, direct[0].TargetPath(), shared)
	assertBranchPresence(t, "direct true", mustBranchValue(t, direct[0], true), presence.Present())
	assertBranchPresence(t, "direct false", mustBranchValue(t, direct[0], false), presence.Absent())
	assertPathEqual(t, direct[1].TargetPath(), falseOnly)
	if _, ok := direct[1].TrueValue(); ok {
		t.Fatalf("false-only direct refinement has true value: %#v", direct[1])
	}
	assertBranchPresence(t, "direct false-only false", mustBranchValue(t, direct[1], false), presence.Absent())

	negated := condition.BranchRefinementsForValue(false)
	if len(negated) != 2 {
		t.Fatalf("negated refinements = %d, want 2: %#v", len(negated), negated)
	}
	assertPathEqual(t, negated[0].TargetPath(), shared)
	assertBranchPresence(t, "negated true", mustBranchValue(t, negated[0], true), presence.Absent())
	assertBranchPresence(t, "negated false", mustBranchValue(t, negated[0], false), presence.Present())
	assertPathEqual(t, negated[1].TargetPath(), falseOnly)
	assertBranchPresence(t, "negated true-only true", mustBranchValue(t, negated[1], true), presence.Absent())
	if _, ok := negated[1].FalseValue(); ok {
		t.Fatalf("false-only negated refinement has false value: %#v", negated[1])
	}
}

func assertBranchPresence(t *testing.T, label string, got ValueRefinement, want presence.Value) {
	t.Helper()
	constraint, ok := got.Constraint()
	if !ok {
		t.Fatalf("%s constraint missing", label)
	}
	if gotPresence := product.PresenceOf(constraint); !presence.Equal(gotPresence, want) {
		t.Fatalf("%s presence = %s, want %s", label, gotPresence, want)
	}
}

func mustBranchValue(t *testing.T, refinement BranchRefinement, edge bool) ValueRefinement {
	t.Helper()
	value, ok := refinement.ValueForEdge(edge)
	if !ok {
		t.Fatalf("missing branch value on edge %v for %#v", edge, refinement)
	}
	return value
}
