package candidates

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestCandidateProvenanceRejectsEqualDenominatorForeignOwners(t *testing.T) {
	first := openCandidateFixture(t, candidateIntegrationSpec())
	firstResult, err := Seal(first.sourceView.Identity(), first.flowView, first.proof,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("first candidates.Seal: %v", err)
	}
	sourceID := first.sourceView.Identity().ContentID()
	flowID := first.flowView.Cold().ContentID()
	staticID := first.staticFinalize.View().ContentID()
	moduleID := first.moduleFinalize.View().ContentID()
	if !Matches(firstResult, sourceID, flowID, staticID, moduleID) {
		t.Fatal("candidate result did not retain its exact Source/Flow identities")
	}
	foreignStaticID := staticID
	foreignStaticID[0] ^= 0xff
	foreignModuleID := moduleID
	foreignModuleID[0] ^= 0xff
	if Matches(firstResult, sourceID, flowID, foreignStaticID, moduleID) || Matches(firstResult, sourceID, flowID, staticID, foreignModuleID) {
		t.Fatal("candidate provenance accepted a foreign Static or Module")
	}

	// Change only an owned Source atom. The foreign fixture keeps every
	// denominator and authored Flow row equal, so a count-only fence cannot
	// distinguish it from the first owner.
	foreignSourceSpec := candidateIntegrationSpec()
	foreignSourceSpec.exactAtoms[0].String = "foreign-source"
	foreignSourceSpec.keys[0] = source.NameKey(candidateTerm(keyspace.FamilyBody, 1), "foreign-source")
	foreignSource := openCandidateFixture(t, foreignSourceSpec)
	foreignSourceID := foreignSource.sourceView.Identity().ContentID()
	if sourceID == foreignSourceID || first.sourceView.Identity().TermCount() != foreignSource.sourceView.Identity().TermCount() {
		t.Fatal("foreign Source fixture did not preserve equal denominator with a distinct identity")
	}
	foreignSourceResult, err := Seal(foreignSource.sourceView.Identity(), foreignSource.flowView, foreignSource.proof,
		foreignSource.staticFinalize.View().ContentID(), foreignSource.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("foreign Source candidates.Seal: %v", err)
	}
	if Matches(firstResult, foreignSourceID, flowID, staticID, moduleID) || Matches(foreignSourceResult, sourceID, flowID, staticID, moduleID) {
		t.Fatal("candidate provenance accepted an equal-denominator foreign Source")
	}

	// Change one authored operator while preserving the complete denominator.
	// This changes only the Flow identity; Source remains the same owner.
	foreignFlowSpec := candidateIntegrationSpec()
	foreignFlowSpec.flow.Operators.Binaries[0].Op = kind.BinarySub
	foreignFlow := openCandidateFixture(t, foreignFlowSpec)
	foreignFlowID := foreignFlow.flowView.Cold().ContentID()
	if sourceID != foreignFlow.sourceView.Identity().ContentID() || flowID == foreignFlowID {
		t.Fatal("foreign Flow fixture did not preserve equal denominator with a distinct identity")
	}
	foreignFlowResult, err := Seal(foreignFlow.sourceView.Identity(), foreignFlow.flowView, foreignFlow.proof,
		foreignFlow.staticFinalize.View().ContentID(), foreignFlow.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("foreign Flow candidates.Seal: %v", err)
	}
	if Matches(firstResult, sourceID, foreignFlowID, staticID, moduleID) || Matches(foreignFlowResult, sourceID, flowID, staticID, moduleID) {
		t.Fatal("candidate provenance accepted an equal-denominator foreign Flow")
	}
}

func TestCandidateProvenanceFailsClosedForNilAndZeroIDs(t *testing.T) {
	result := &Result{
		// Keep plausible projection data so this tests the provenance fence,
		// rather than an empty-result special case.
		buckets: bucketStore{unaryNumeric: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyUnary, 1)}},
		classes: classStore{unaryClass: []uint8{unaryNumericCandidate}},
	}
	var sourceID, flowID identity.ContentID
	sourceID[0] = 0x11
	flowID[0] = 0x22
	var staticID, moduleID identity.ContentID
	staticID[0] = 0x33
	moduleID[0] = 0x44
	if Matches(nil, sourceID, flowID, staticID, moduleID) || Matches(result, sourceID, flowID, staticID, moduleID) ||
		Matches(result, identity.ContentID{}, flowID, staticID, moduleID) ||
		Matches(result, sourceID, identity.ContentID{}, staticID, moduleID) ||
		Matches(result, sourceID, flowID, identity.ContentID{}, moduleID) ||
		Matches(result, sourceID, flowID, staticID, identity.ContentID{}) {
		t.Fatal("candidate provenance accepted nil or zero/unavailable owner identity")
	}
}
