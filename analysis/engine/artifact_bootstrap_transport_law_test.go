package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func bootstrapTransportID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func bootstrapTransportCapability(t testing.TB, ordinal uint64) RuleSlotCapability {
	t.Helper()
	return framingLawCapability(t.(*testing.T), ruleCapabilityLink, ordinal, false)
}

// TestLinkBootstrapTransportsOnlyValueAndHeapToMountedInitialPoints proves
// the Link bootstrap witness keeps separate occurrence namespaces. A point or
// occurrence outside the two declared catalogs never becomes transportable.
func TestLinkBootstrapTransportsOnlyValueAndHeapToMountedInitialPoints(t *testing.T) {
	value, heap := bootstrapTransportID(11), bootstrapTransportID(12)
	valueCap, heapCap := bootstrapTransportCapability(t, 0), bootstrapTransportCapability(t, 1)
	bootstrap, ok := NewProgramBootstrap(
		bootstrapTransportID(1), bootstrapTransportID(2),
		ProgramBootstrapCatalog{Capability: valueCap, Occurrences: []identity.ContentID{value}},
		ProgramBootstrapCatalog{Capability: heapCap, Occurrences: []identity.ContentID{heap}},
	)
	if !ok || !bootstrap.witness.Available() || bootstrap.witness.OccurrenceCount() != 2 {
		t.Fatal("bootstrap transport witness did not seal")
	}
	for occurrence, want := range map[identity.ContentID]RuleSlotCapability{value: valueCap, heap: heapCap} {
		got, found := bootstrap.witness.capabilityFor(occurrence)
		if !found || got != want {
			t.Fatalf("bootstrap occurrence %x=%v/%t, want %v/true", occurrence, got, found, want)
		}
	}
	if _, found := bootstrap.witness.capabilityFor(bootstrapTransportID(99)); found {
		t.Fatal("bootstrap transport admitted an undeclared occurrence")
	}
}

// TestLinkBootstrapTransportRejectsForeignCapabilityAtSnapshot proves the
// capability kind is checked at the public ProgramBootstrap boundary.
func TestLinkBootstrapTransportRejectsForeignCapabilityAtSnapshot(t *testing.T) {
	mounted := framingLawCapability(t, ruleCapabilityMounted, 3, false)
	if bootstrap, ok := NewProgramBootstrap(bootstrapTransportID(3), bootstrapTransportID(4), ProgramBootstrapCatalog{Capability: mounted, Occurrences: []identity.ContentID{bootstrapTransportID(5)}}); ok || bootstrap.witness.Available() {
		t.Fatal("mounted capability crossed into the Link bootstrap witness")
	}
}

// TestLinkBootstrapTransportRejectsMissingOrAmbiguousMountedInitialPoint
// proves unavailable points, empty owners and duplicate namespaces fail before
// any program geometry is published.
func TestLinkBootstrapTransportRejectsMissingOrAmbiguousMountedInitialPoint(t *testing.T) {
	capability := bootstrapTransportCapability(t, 4)
	if _, ok := NewProgramBootstrap(identity.ContentID{}, bootstrapTransportID(6)); ok {
		t.Fatal("unavailable bootstrap owner admitted")
	}
	if _, ok := NewProgramBootstrap(bootstrapTransportID(7), identity.ContentID{}); ok {
		t.Fatal("unavailable bootstrap point admitted")
	}
	shared := bootstrapTransportID(8)
	if _, ok := NewProgramBootstrap(bootstrapTransportID(9), bootstrapTransportID(10),
		ProgramBootstrapCatalog{Capability: capability, Occurrences: []identity.ContentID{shared}},
		ProgramBootstrapCatalog{Capability: bootstrapTransportCapability(t, 5), Occurrences: []identity.ContentID{shared}},
	); ok {
		t.Fatal("duplicate bootstrap occurrence admitted under two capabilities")
	}
}

// TestLinkBootstrapCatalogCopiesOccurrenceNamespacesAcrossReseal proves the
// caller's catalog is copied into each immutable witness and that resealing
// does not mutate the prior witness. Transport selection is deliberately not
// part of this catalog law; the sealed Binding owns that smaller set.
func TestLinkBootstrapCatalogCopiesOccurrenceNamespacesAcrossReseal(t *testing.T) {
	first, second := bootstrapTransportCapability(t, 6), bootstrapTransportCapability(t, 7)
	occurrences := []identity.ContentID{bootstrapTransportID(21), bootstrapTransportID(22)}
	left, leftOK := NewProgramBootstrap(bootstrapTransportID(23), bootstrapTransportID(24),
		ProgramBootstrapCatalog{Capability: first, Occurrences: occurrences[:1]},
		ProgramBootstrapCatalog{Capability: second, Occurrences: occurrences[1:]})
	right, rightOK := NewProgramBootstrap(bootstrapTransportID(23), bootstrapTransportID(24),
		ProgramBootstrapCatalog{Capability: first, Occurrences: occurrences[:1]},
		ProgramBootstrapCatalog{Capability: second, Occurrences: occurrences[1:]})
	if !leftOK || !rightOK || left.witness.catalogCapabilityCount() != 2 || right.witness.catalogCapabilityCount() != 2 {
		t.Fatal("bootstrap catalog did not seal")
	}
	for index := 0; index < 2; index++ {
		leftCapability, leftFound := left.witness.catalogCapabilityAt(index)
		rightCapability, rightFound := right.witness.catalogCapabilityAt(index)
		if !leftFound || !rightFound || leftCapability != rightCapability {
			t.Fatalf("catalog index %d changed across reseal", index)
		}
	}
	leftFirst, leftFirstOK := left.witness.OccurrenceAt(0)
	rightFirst, rightFirstOK := right.witness.OccurrenceAt(0)
	leftSecond, leftSecondOK := left.witness.OccurrenceAt(1)
	rightSecond, rightSecondOK := right.witness.OccurrenceAt(1)
	if !leftFirstOK || !rightFirstOK || !leftSecondOK || !rightSecondOK || leftFirst != rightFirst || leftSecond != rightSecond {
		t.Fatal("bootstrap occurrence order changed across reseal")
	}
}

// TestLinkBootstrapValueAndHeapProducersReachMountedInitialAfterReleaseAndRevision
// keeps the runtime half of the law on the current committed-program seam:
// both fresh seals solve the same mounted producer geometry independently.
func TestLinkBootstrapValueAndHeapProducersReachMountedInitialAfterReleaseAndRevision(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	first, failure, ok := fixture.graph.Seal(nil)
	if !ok || first == nil {
		t.Fatalf("first mounted seal=%v", failure)
	}
	firstState, firstStatus := first.Solve(context.Background())
	second, failure, ok := fixture.graph.Seal(nil)
	if !ok || second == nil {
		t.Fatalf("revision mounted seal=%v", failure)
	}
	secondState, secondStatus := second.Solve(context.Background())
	if firstState == nil || firstStatus != SolveComplete || secondState == nil || secondStatus != SolveComplete {
		t.Fatalf("mounted solves=%t/%v and %t/%v", firstState != nil, firstStatus, secondState != nil, secondStatus)
	}
	if !first.ownsCompletedState(firstState) || !second.ownsCompletedState(secondState) || first.ownsCompletedState(secondState) || second.ownsCompletedState(firstState) {
		t.Fatal("mounted bootstrap revision crossed its publication fence")
	}
}
