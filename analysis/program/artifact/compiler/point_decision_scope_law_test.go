package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func scopeDecision(value byte) pointDecisionDraft {
	atom, _ := region.NewAtom(valuesLawID(value))
	return pointDecisionDraft{semantic: valuesLawID(value), atom: atom}
}

func TestColdPointPlanesShareDecisionScopesInSortedRowOrder(t *testing.T) {
	baseA, stageA := valuesLawID(20), valuesLawID(10)
	baseB, stageB := valuesLawID(40), valuesLawID(30)
	rows := []pointDraft{
		{id: stageA, decisionScope: baseA},
		{id: baseA, decisionScope: baseA, decisions: []pointDecisionDraft{scopeDecision(2), scopeDecision(4)}, initial: true},
		{id: stageB, decisionScope: baseB},
		{id: baseB, decisionScope: baseB, decisions: []pointDecisionDraft{scopeDecision(3)}},
	}
	points, decisions, ok := coldPointPlanes(rows)
	if !ok || len(points) != len(rows) || len(decisions) != 3 {
		t.Fatalf("shared point planes ok=%v points=%d decisions=%d", ok, len(points), len(decisions))
	}
	for index, row := range rows {
		if points[index].ID() != row.id {
			t.Fatalf("point %d ID = %v, want %v", index, points[index].ID(), row.id)
		}
		if points[index].Initial() != row.initial {
			t.Fatalf("point %d initial = %v, want %v", index, points[index].Initial(), row.initial)
		}
		if points[index].ScopeID() != row.decisionScope {
			t.Fatalf("point %d scope = %v, want exact owner %v", index, points[index].ScopeID(), row.decisionScope)
		}
	}
	firstOffset, firstCount, firstOK := points[0].DecisionSpan()
	secondOffset, secondCount, secondOK := points[1].DecisionSpan()
	thirdOffset, thirdCount, thirdOK := points[2].DecisionSpan()
	fourthOffset, fourthCount, fourthOK := points[3].DecisionSpan()
	if !firstOK || !secondOK || !thirdOK || !fourthOK ||
		firstOffset != secondOffset || firstCount != secondCount ||
		thirdOffset != fourthOffset || thirdCount != fourthCount ||
		firstOffset != 0 || firstCount != 2 || thirdOffset != 2 || thirdCount != 1 {
		t.Fatalf("scope spans = (%d,%d), (%d,%d), (%d,%d), (%d,%d)", firstOffset, firstCount, secondOffset, secondCount, thirdOffset, thirdCount, fourthOffset, fourthCount)
	}
	want := []identity.ContentID{valuesLawID(2), valuesLawID(4), valuesLawID(3)}
	for index, row := range decisions {
		if row.ID() != want[index] {
			t.Fatalf("decision %d = %v, want %v", index, row.ID(), want[index])
		}
	}
}

func TestColdPointPlanesRejectUnownedOrCopiedDecisionScopes(t *testing.T) {
	owner, stage := valuesLawID(50), valuesLawID(51)
	if _, _, ok := coldPointPlanes([]pointDraft{{id: stage, decisionScope: owner}}); ok {
		t.Fatal("scope without its base owner was accepted")
	}
	if _, _, ok := coldPointPlanes([]pointDraft{
		{id: owner, decisionScope: owner, decisions: []pointDecisionDraft{scopeDecision(5)}},
		{id: stage, decisionScope: owner, decisions: []pointDecisionDraft{scopeDecision(5)}},
	}); ok {
		t.Fatal("synthetic row carrying a copied decision vector was accepted")
	}
}
