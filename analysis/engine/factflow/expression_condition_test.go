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

func TestExpressionConditionBranchPathRelationsForValue(t *testing.T) {
	left := path.NewPath(symbol.ID(3), "left")
	right := path.NewPath(symbol.ID(4), "right")
	other := path.NewPath(symbol.ID(5), "other")
	condition := NewExpressionCondition(
		nil,
		nil,
		[]PostconditionPathRelation{NewPostconditionPathEquality(left, right)},
		[]PostconditionPathRelation{NewPostconditionPathEquality(left, other)},
	)

	direct := condition.BranchPathRelationsForValue(true)
	if len(direct) != 4 {
		t.Fatalf("direct relations = %d, want 4: %#v", len(direct), direct)
	}
	assertBranchPathRelation(t, direct[0], BranchPathRelationEqual, left, right, true, false)
	assertBranchPathRelation(t, direct[1], BranchPathRelationNotEqual, left, right, false, true)
	assertBranchPathRelation(t, direct[2], BranchPathRelationEqual, left, other, false, true)
	assertBranchPathRelation(t, direct[3], BranchPathRelationNotEqual, left, other, true, false)

	negated := condition.BranchPathRelationsForValue(false)
	if len(negated) != 4 {
		t.Fatalf("negated relations = %d, want 4: %#v", len(negated), negated)
	}
	assertBranchPathRelation(t, negated[0], BranchPathRelationEqual, left, other, true, false)
	assertBranchPathRelation(t, negated[1], BranchPathRelationNotEqual, left, other, false, true)
	assertBranchPathRelation(t, negated[2], BranchPathRelationEqual, left, right, false, true)
	assertBranchPathRelation(t, negated[3], BranchPathRelationNotEqual, left, right, true, false)
}

func TestExpressionConditionBranchPathEvidenceForValue(t *testing.T) {
	left := path.NewPath(symbol.ID(6), "left")
	right := path.NewPath(symbol.ID(7), "right")
	condition := NewExpressionCondition(
		nil,
		nil,
		[]PostconditionPathRelation{NewPostconditionPathEquality(left, right)},
		nil,
	)

	direct := condition.BranchPathEvidenceForValue(true)
	if len(direct) != 2 {
		t.Fatalf("direct evidence = %d, want 2: %#v", len(direct), direct)
	}
	assertBranchPathEvidence(t, direct[0], BranchPathEvidenceEqual, left, right, true, false)
	assertBranchPathEvidence(t, direct[1], BranchPathEvidenceNotEqual, left, right, false, true)

	negated := condition.BranchPathEvidenceForValue(false)
	if len(negated) != 2 {
		t.Fatalf("negated evidence = %d, want 2: %#v", len(negated), negated)
	}
	assertBranchPathEvidence(t, negated[0], BranchPathEvidenceEqual, left, right, false, true)
	assertBranchPathEvidence(t, negated[1], BranchPathEvidenceNotEqual, left, right, true, false)
}

func assertBranchPathRelation(
	t *testing.T,
	got BranchPathRelation,
	wantKind BranchPathRelationKind,
	wantLeft path.Path,
	wantRight path.Path,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	if got.Kind() != wantKind {
		t.Fatalf("relation kind = %v, want %v", got.Kind(), wantKind)
	}
	assertPathEqual(t, got.LeftPath(), wantLeft)
	assertPathEqual(t, got.RightPath(), wantRight)
	if got.ActiveOnEdge(true) != wantTrue || got.ActiveOnEdge(false) != wantFalse {
		t.Fatalf("relation edges true/false = %v/%v, want %v/%v", got.ActiveOnEdge(true), got.ActiveOnEdge(false), wantTrue, wantFalse)
	}
}

func assertBranchPathEvidence(
	t *testing.T,
	got BranchPathEvidence,
	wantKind BranchPathEvidenceKind,
	wantLeft path.Path,
	wantRight path.Path,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	if got.Kind() != wantKind {
		t.Fatalf("evidence kind = %v, want %v", got.Kind(), wantKind)
	}
	assertPathEqual(t, got.Path(), wantLeft)
	other, ok := got.OtherPath()
	if !ok {
		t.Fatalf("evidence other path missing for %#v", got)
	}
	assertPathEqual(t, other, wantRight)
	if got.ActiveOnEdge(true) != wantTrue || got.ActiveOnEdge(false) != wantFalse {
		t.Fatalf("evidence edges true/false = %v/%v, want %v/%v", got.ActiveOnEdge(true), got.ActiveOnEdge(false), wantTrue, wantFalse)
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
