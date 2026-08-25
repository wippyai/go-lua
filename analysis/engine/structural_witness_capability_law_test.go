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

// TestStructuralWitnessCapabilityResolvesCapabilityOccurrenceClaim proves
// that a Link-global witness admits the same owner-issued occurrence
// independently in two capability namespaces, while a foreign pair is
// refused without a mount or domain-role interpretation.
func TestStructuralWitnessCapabilityResolvesCapabilityOccurrenceClaim(t *testing.T) {
	shared := witnessLawID(41)
	first, second, foreign := witnessLawCapability(t, 0), witnessLawCapability(t, 1), witnessLawCapability(t, 2)
	witness, ok := NewLinkBootstrapWitnessByCapability(
		witnessLawID(1),
		LinkBootstrapPoint{PointID: witnessLawID(2), Known: true, Initial: true},
		LinkBootstrapCatalog{Capability: first, Occurrences: []identity.ContentID{shared}},
		LinkBootstrapCatalog{Capability: second, Occurrences: []identity.ContentID{shared}},
	)
	if !ok || !witness.Available() || witness.claimCount() != 2 {
		t.Fatal("capability-scoped witness did not seal")
	}
	if !witness.admits(first, shared) || !witness.admits(second, shared) {
		t.Fatal("same occurrence was not admitted independently by both capabilities")
	}
	if witness.admits(foreign, shared) || witness.admits(first, witnessLawID(99)) {
		t.Fatal("foreign capability+occurrence address was admitted")
	}
}

// TestStructuralWitnessCapabilityRejectsDuplicateCapabilityOccurrenceClaim
// proves that the same address cannot be repeated inside one capability
// namespace.
func TestStructuralWitnessCapabilityRejectsDuplicateCapabilityOccurrenceClaim(t *testing.T) {
	shared := witnessLawID(51)
	first := witnessLawCapability(t, 2)
	if witness, ok := NewLinkBootstrapWitnessByCapability(
		witnessLawID(3), LinkBootstrapPoint{PointID: witnessLawID(4), Known: true, Initial: true},
		LinkBootstrapCatalog{Capability: first, Occurrences: []identity.ContentID{shared, shared}},
	); ok || witness.Available() {
		t.Fatal("duplicate capability+occurrence address was admitted")
	}
}

// TestStructuralWitnessCapabilityRejectsDuplicateCapabilityCatalog proves the
// public witness constructor refuses duplicate capability catalogs before a
// witness is published.
func TestStructuralWitnessCapabilityRejectsDuplicateCapabilityCatalog(t *testing.T) {
	shared := witnessLawID(61)
	first := witnessLawCapability(t, 4)
	if witness, ok := NewLinkBootstrapWitnessByCapability(
		witnessLawID(5), LinkBootstrapPoint{PointID: witnessLawID(6), Known: true, Initial: true},
		LinkBootstrapCatalog{Capability: first, Occurrences: []identity.ContentID{shared}},
		LinkBootstrapCatalog{Capability: first, Occurrences: []identity.ContentID{witnessLawID(62)}},
	); ok || witness.Available() {
		t.Fatal("duplicate capability catalog was admitted")
	}
}
