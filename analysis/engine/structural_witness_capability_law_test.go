package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestStructuralWitnessCapabilityResolvesDisjointClaim proves a Link-global
// occurrence resolves to the exact capability that admitted it, and that the
// answer does not depend on the unordered capability namespace walk.
func TestStructuralWitnessCapabilityResolvesDisjointClaim(t *testing.T) {
	owner := newBootstrapTransportLawOwner(t)
	value := bootstrapTransportLawID(9, 40)
	heap := bootstrapTransportLawID(9, 41)
	witness, ok := NewLinkBootstrapWitnessByCapability(
		bootstrapTransportLawID(9, 42),
		LinkBootstrapPoint{PointID: bootstrapTransportLawID(9, 43), Known: true, Initial: true},
		owner.value, []identity.ContentID{value},
		owner.heap, []identity.ContentID{heap},
	)
	if !ok || !witness.Available() {
		t.Fatal("disjoint capability witness")
	}
	for repeat := 0; repeat < 64; repeat++ {
		valueCapability, valueOK := witness.capabilityFor(value)
		heapCapability, heapOK := witness.capabilityFor(heap)
		if !valueOK || valueCapability != owner.value {
			t.Fatalf("repeat %d value capability", repeat)
		}
		if !heapOK || heapCapability != owner.heap {
			t.Fatalf("repeat %d heap capability", repeat)
		}
	}
}

// TestStructuralWitnessCapabilityRejectsCrossSlotClaim proves an occurrence
// claimed by more than one capability is refused. Capability namespaces are
// disjoint by contract, so a cross-slot claim has no legitimate winner.
func TestStructuralWitnessCapabilityRejectsCrossSlotClaim(t *testing.T) {
	owner := newBootstrapTransportLawOwner(t)
	shared := bootstrapTransportLawID(9, 50)
	witness := LinkBootstrapWitness{
		owner:       bootstrapTransportLawID(9, 51),
		point:       LinkBootstrapPoint{PointID: bootstrapTransportLawID(9, 52), Known: true, Initial: true},
		occurrences: []identity.ContentID{shared},
		byCapability: map[RuleSlotCapability]map[identity.ContentID]struct{}{
			owner.value: {shared: struct{}{}},
			owner.heap:  {shared: struct{}{}},
		},
		transportCapabilities: []RuleSlotCapability{owner.value, owner.heap},
	}
	if !witness.Available() {
		t.Fatal("cross-slot witness fixture")
	}
	for repeat := 0; repeat < 64; repeat++ {
		capability, ok := witness.capabilityFor(shared)
		if ok || capability != (RuleSlotCapability{}) {
			t.Fatalf("repeat %d: cross-slot occurrence resolved to a capability", repeat)
		}
	}
}

// TestStructuralWitnessCapabilityRejectsCrossSlotAdmission proves the sealing
// constructor is the first authority refusing a cross-slot occurrence.
func TestStructuralWitnessCapabilityRejectsCrossSlotAdmission(t *testing.T) {
	owner := newBootstrapTransportLawOwner(t)
	shared := bootstrapTransportLawID(9, 60)
	witness, ok := NewLinkBootstrapWitnessByCapability(
		bootstrapTransportLawID(9, 61),
		LinkBootstrapPoint{PointID: bootstrapTransportLawID(9, 62), Known: true, Initial: true},
		owner.value, []identity.ContentID{shared},
		owner.heap, []identity.ContentID{shared},
	)
	if ok || witness.Available() {
		t.Fatal("cross-slot occurrence admitted")
	}
}
