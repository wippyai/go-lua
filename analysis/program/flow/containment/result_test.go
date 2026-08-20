package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestResultNilQueriesFailClosed(t *testing.T) {
	var result *Result

	if got := result.Count(); got != 0 {
		t.Fatalf("nil Count() = %d, want 0", got)
	}
	if term, ok := result.At(0); ok || term != 0 {
		t.Fatalf("nil At(0) = %v/%v, want 0/false", term, ok)
	}
	if parent, ok := result.Parent(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok || parent != 0 {
		t.Fatalf("nil Parent() = %v/%v, want 0/false", parent, ok)
	}
	if result.Contains(keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyBody, 1)) {
		t.Fatal("nil Contains() returned true")
	}
	if result.Static(keyspace.MakeTerm(keyspace.FamilyBody, 1)) {
		t.Fatal("nil Static() returned true")
	}
}

func TestResultUnavailableQueriesFailClosed(t *testing.T) {
	result, err := buildKernel(kernelInput{
		counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1},
		roots:  []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 1)},
	})
	if err != nil {
		t.Fatalf("buildKernel() error = %v", err)
	}

	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if result.Count() != 0 {
		t.Fatalf("zero-provenance Count() = %d, want 0", result.Count())
	}
	if term, ok := result.At(0); ok || term != 0 {
		t.Fatalf("zero-provenance At(0) = %v/%v, want 0/false", term, ok)
	}
	for _, term := range []keyspace.Term{
		0,
		keyspace.MakeTerm(keyspace.FamilyBody, 2),
		keyspace.MakeTerm(keyspace.FamilyCell, 1),
	} {
		if parent, ok := result.Parent(term); ok || parent != 0 {
			t.Errorf("Parent(%v) = %v/%v, want 0/false", term, parent, ok)
		}
		if result.Contains(term, body) || result.Contains(body, term) {
			t.Errorf("Contains accepted unavailable term %v", term)
		}
		if result.Static(term) {
			t.Errorf("Static(%v) = true, want false", term)
		}
	}
}

func TestKernelFailureReturnsNilProof(t *testing.T) {
	result, err := buildKernel(kernelInput{
		counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1},
	})
	if err == nil {
		t.Fatal("buildKernel() accepted a forest without a root")
	}
	if result != nil {
		t.Fatal("buildKernel() returned a proof with an error")
	}
}

func TestResultMatchesExactFourOwnerIdentities(t *testing.T) {
	counts := countsFor(c(keyspace.FamilyBody, 1))
	fixture := newProofFixture(t, proofSpec{counts: counts, module: emptyModule(t)})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	sourceID := fixture.preimage.Identity().ContentID()
	flowID := fixture.flowView.ContentID()
	moduleID := fixture.moduleView.ContentID()
	staticID := fixture.staticView.ContentID()
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("Result did not match its exact Source/Flow/Module/Static owners")
	}
	foreignSource := sourceID
	foreignSource[0] ^= 0xff
	if Matches(result, foreignSource, flowID, staticID, moduleID) {
		t.Fatal("Result accepted an equal-cardinality foreign Source identity")
	}
	foreignFlow := flowID
	foreignFlow[0] ^= 0xff
	if Matches(result, sourceID, foreignFlow, staticID, moduleID) {
		t.Fatal("Result accepted an equal-cardinality foreign Flow identity")
	}

	// The fixture's dense counts and rows are unchanged; only each foreign
	// owner's scalar identity differs, which is the splice StaticCheck must
	// reject without retaining an owner view.
	foreignStatic := staticID
	foreignStatic[0] ^= 0xff
	if Matches(result, sourceID, flowID, foreignStatic, moduleID) {
		t.Fatal("Result accepted an equal-cardinality foreign Static identity")
	}
	foreignModule := moduleID
	foreignModule[0] ^= 0xff
	if Matches(result, sourceID, flowID, staticID, foreignModule) {
		t.Fatal("Result accepted an equal-cardinality foreign Module identity")
	}
	if Matches(nil, sourceID, flowID, staticID, moduleID) ||
		Matches(result, sourceID, flowID, identity.ContentID{}, moduleID) ||
		Matches(result, sourceID, flowID, staticID, identity.ContentID{}) {
		t.Fatal("Result accepted an unavailable owner identity")
	}
}

func TestProveRejectsUnavailableOrForeignEqualCardinalityBodyBinding(t *testing.T) {
	counts := countsFor(c(keyspace.FamilyBody, 1))
	fixture := newProofFixture(t, proofSpec{counts: counts, module: emptyModule(t)})
	foreign := newProofFixture(t, proofSpec{
		counts: counts,
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "foreign"}},
		module: emptyModule(t),
	})
	if _, _, err := Prove(fixture.preimage, fixture.staticView, fixture.flowView, nil, fixture.binding, fixture.moduleView, fixture.entry); err == nil {
		t.Fatal("Prove accepted a nil Body before its owner fence")
	}
	if _, _, err := Prove(fixture.preimage, fixture.staticView, fixture.flowView, foreign.bodies, fixture.binding, fixture.moduleView, fixture.entry); err == nil {
		t.Fatal("Prove accepted a foreign equal-cardinality Body")
	}
	if _, _, err := Prove(fixture.preimage, fixture.staticView, fixture.flowView, fixture.bodies, foreign.binding, fixture.moduleView, fixture.entry); err == nil {
		t.Fatal("Prove accepted a foreign equal-cardinality Binding")
	}
}
