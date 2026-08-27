package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func TestAdmitPointDecisionAppendsToSharedOwnerAndCanonicalizesDuplicates(t *testing.T) {
	owner, stage := valuesLawID(20), valuesLawID(10)
	decision := valuesLawID(30)
	atom, atomOK := region.NewAtom(decision)
	if !atomOK {
		t.Fatal("decision atom")
	}
	transaction := compiler{pointGeometry: map[identity.ContentID]pointDraft{
		owner: {id: owner, decisionScope: owner},
		stage: {id: stage, decisionScope: owner},
	}}
	if !transaction.admitPointDecision(stage, decision, atom) || !transaction.admitPointDecision(owner, decision, atom) {
		t.Fatal("valid shared-scope decisions were not admitted")
	}
	ownerRow := transaction.pointGeometry[owner]
	if len(ownerRow.decisions) != 2 {
		t.Fatalf("owner received %d decisions, want 2 direct admissions", len(ownerRow.decisions))
	}
	if len(transaction.pointGeometry[stage].decisions) != 0 {
		t.Fatal("shared synthetic point received a copied decision vector")
	}
	if failure := transaction.canonicalizePointDecisionsFailure(); failure.Available() {
		t.Fatalf("canonicalization failed after valid duplicate admissions: %v", failure)
	}
	ownerRow = transaction.pointGeometry[owner]
	if len(ownerRow.decisions) != 1 || ownerRow.decisions[0].semantic != decision || ownerRow.decisions[0].atom != atom {
		t.Fatalf("owner decisions after dedupe = %v, want [%v]", ownerRow.decisions, decision)
	}
}

func TestCanonicalizePointDecisionsSortsEverySharedScopeDeterministically(t *testing.T) {
	ownerA, stageA := valuesLawID(40), valuesLawID(41)
	ownerB, stageB := valuesLawID(50), valuesLawID(51)
	transaction := compiler{pointGeometry: map[identity.ContentID]pointDraft{
		stageA: {id: stageA, decisionScope: ownerA},
		ownerA: {id: ownerA, decisionScope: ownerA, decisions: []pointDecisionDraft{pointDecision(4), pointDecision(1), pointDecision(4)}},
		stageB: {id: stageB, decisionScope: ownerB},
		ownerB: {id: ownerB, decisionScope: ownerB, decisions: []pointDecisionDraft{pointDecision(3), pointDecision(2)}},
	}}
	if failure := transaction.canonicalizePointDecisionsFailure(); failure.Available() {
		t.Fatalf("canonicalization failed: %v", failure)
	}
	if got := transaction.pointGeometry[ownerA].decisions; len(got) != 2 || got[0].semantic != valuesLawID(1) || got[1].semantic != valuesLawID(4) {
		t.Fatalf("owner A decisions = %v, want sorted unique [01 04]", got)
	}
	if got := transaction.pointGeometry[ownerB].decisions; len(got) != 2 || got[0].semantic != valuesLawID(2) || got[1].semantic != valuesLawID(3) {
		t.Fatalf("owner B decisions = %v, want sorted [02 03]", got)
	}
	if len(transaction.pointGeometry[stageA].decisions) != 0 || len(transaction.pointGeometry[stageB].decisions) != 0 {
		t.Fatal("canonicalization copied decisions onto shared synthetic points")
	}
}

func TestPointDecisionAdmissionAndCanonicalizationFailClosed(t *testing.T) {
	owner, stage := valuesLawID(60), valuesLawID(61)
	transaction := compiler{pointGeometry: map[identity.ContentID]pointDraft{
		owner: {id: owner, decisionScope: owner},
		stage: {id: stage, decisionScope: owner},
	}}
	if transaction.admitPointDecision(stage, identity.ContentID{}, region.Atom{}) {
		t.Fatal("invalid decision was admitted")
	}
	if len(transaction.pointGeometry[owner].decisions) != 0 {
		t.Fatal("invalid admission mutated the owner")
	}
	transaction.pointGeometry[owner] = pointDraft{
		id: owner, decisionScope: owner, decisions: []pointDecisionDraft{{semantic: valuesLawID(2)}, {semantic: identity.ContentID{}}, pointDecision(1)},
	}
	if failure := transaction.canonicalizePointDecisionsFailure(); !failure.Available() || failure.Reason() != CompileReasonRouteGuard {
		t.Fatalf("invalid stored decision did not fail closed: %v", failure)
	}
}
