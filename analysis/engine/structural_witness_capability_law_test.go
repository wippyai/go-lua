package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func witnessLawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func witnessLawCapability(t testing.TB, ordinal uint64) RuleSlotCapability {
	t.Helper()
	return framingLawCapability(t.(*testing.T), ruleCapabilityLink, ordinal, false)
}

// TestStructuralWitnessCapabilityResolvesDisjointClaim proves that a
// Link-global witness resolves each occurrence through exactly its issuing
// capability namespace, without a mount or domain-role interpretation.
func TestStructuralWitnessCapabilityResolvesDisjointClaim(t *testing.T) {
	value, heap := witnessLawID(41), witnessLawID(42)
	first, second := witnessLawCapability(t, 0), witnessLawCapability(t, 1)
	witness, ok := NewLinkBootstrapWitnessByCapability(
		witnessLawID(1),
		LinkBootstrapPoint{PointID: witnessLawID(2), Known: true, Initial: true},
		LinkBootstrapCatalog{Capability: first, Occurrences: []identity.ContentID{value}},
		LinkBootstrapCatalog{Capability: second, Occurrences: []identity.ContentID{heap}},
	)
	if !ok || !witness.Available() || witness.OccurrenceCount() != 2 {
		t.Fatal("disjoint witness did not seal")
	}
	for occurrence, want := range map[identity.ContentID]RuleSlotCapability{value: first, heap: second} {
		got, found := witness.capabilityFor(occurrence)
		if !found || got != want {
			t.Fatalf("occurrence %x resolved to %v/%t, want %v/true", occurrence, got, found, want)
		}
	}
	if _, found := witness.capabilityFor(witnessLawID(99)); found {
		t.Fatal("unclaimed occurrence resolved through a witness")
	}
}

// TestStructuralWitnessCapabilityRejectsCrossSlotClaim proves that an
// occurrence cannot be interpreted by two slot capabilities, even if a
// malformed in-memory witness is presented to the resolver.
func TestStructuralWitnessCapabilityRejectsCrossSlotClaim(t *testing.T) {
	shared := witnessLawID(51)
	first, second := witnessLawCapability(t, 2), witnessLawCapability(t, 3)
	witness, ok := NewLinkBootstrapWitnessByCapability(
		witnessLawID(3), LinkBootstrapPoint{PointID: witnessLawID(4), Known: true, Initial: true},
		LinkBootstrapCatalog{Capability: first, Occurrences: []identity.ContentID{shared}},
		LinkBootstrapCatalog{Capability: second, Occurrences: []identity.ContentID{witnessLawID(52)}},
	)
	if !ok {
		t.Fatal("base witness")
	}
	witness.byCapability[second][shared] = struct{}{}
	if capability, found := witness.capabilityFor(shared); found || capability.Available() {
		t.Fatal("cross-slot occurrence claim was resolved")
	}
}

// TestStructuralWitnessCapabilityRejectsCrossSlotAdmission proves the public
// witness constructor refuses duplicate occurrence claims before a witness is
// published.
func TestStructuralWitnessCapabilityRejectsCrossSlotAdmission(t *testing.T) {
	shared := witnessLawID(61)
	first, second := witnessLawCapability(t, 4), witnessLawCapability(t, 5)
	if witness, ok := NewLinkBootstrapWitnessByCapability(
		witnessLawID(5), LinkBootstrapPoint{PointID: witnessLawID(6), Known: true, Initial: true},
		LinkBootstrapCatalog{Capability: first, Occurrences: []identity.ContentID{shared}},
		LinkBootstrapCatalog{Capability: second, Occurrences: []identity.ContentID{shared}},
	); ok || witness.Available() {
		t.Fatal("duplicate cross-slot occurrence claim was admitted")
	}
}
