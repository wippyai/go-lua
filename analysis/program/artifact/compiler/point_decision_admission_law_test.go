package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestAdmitPointDecisionAppendsToSharedOwnerAndCanonicalizesDuplicates(t *testing.T) {
	owner, stage := valuesLawID(20), valuesLawID(10)
	decision := valuesLawID(30)
	transaction := compiler{pointGeometry: map[identity.ContentID]pointDraft{
		owner: {id: owner, decisionScope: owner},
		stage: {id: stage, decisionScope: owner},
	}}
	if !transaction.admitPointDecision(stage, decision) || !transaction.admitPointDecision(owner, decision) {
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
	if len(ownerRow.decisions) != 1 || ownerRow.decisions[0] != decision {
		t.Fatalf("owner decisions after dedupe = %v, want [%v]", ownerRow.decisions, decision)
	}
}

func TestCanonicalizePointDecisionsSortsEverySharedScopeDeterministically(t *testing.T) {
	ownerA, stageA := valuesLawID(40), valuesLawID(41)
	ownerB, stageB := valuesLawID(50), valuesLawID(51)
	transaction := compiler{pointGeometry: map[identity.ContentID]pointDraft{
		stageA: {id: stageA, decisionScope: ownerA},
		ownerA: {id: ownerA, decisionScope: ownerA, decisions: []identity.ContentID{valuesLawID(4), valuesLawID(1), valuesLawID(4)}},
		stageB: {id: stageB, decisionScope: ownerB},
		ownerB: {id: ownerB, decisionScope: ownerB, decisions: []identity.ContentID{valuesLawID(3), valuesLawID(2)}},
	}}
	if failure := transaction.canonicalizePointDecisionsFailure(); failure.Available() {
		t.Fatalf("canonicalization failed: %v", failure)
	}
	if got := transaction.pointGeometry[ownerA].decisions; len(got) != 2 || got[0] != valuesLawID(1) || got[1] != valuesLawID(4) {
		t.Fatalf("owner A decisions = %v, want sorted unique [01 04]", got)
	}
	if got := transaction.pointGeometry[ownerB].decisions; len(got) != 2 || got[0] != valuesLawID(2) || got[1] != valuesLawID(3) {
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
	if transaction.admitPointDecision(stage, identity.ContentID{}) {
		t.Fatal("invalid decision was admitted")
	}
	if len(transaction.pointGeometry[owner].decisions) != 0 {
		t.Fatal("invalid admission mutated the owner")
	}
	transaction.pointGeometry[owner] = pointDraft{
		id: owner, decisionScope: owner, decisions: []identity.ContentID{valuesLawID(2), identity.ContentID{}, valuesLawID(1)},
	}
	if failure := transaction.canonicalizePointDecisionsFailure(); !failure.Available() || failure.Reason() != CompileReasonRouteGuard {
		t.Fatalf("invalid stored decision did not fail closed: %v", failure)
	}
}
