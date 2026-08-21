package publicationescape

import (
	"testing"

	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

var (
	placementAtomClassificationSink placementdomain.AtomClassification
	placementAtomClassificationOK   bool
)

// TestPlacementAtomClassificationKeepsBootAllocationAndOpaqueDistinct pins
// the precision boundary shared by every Value-to-Placement route consumer:
// an exact Boot handle has no local route, an exact allocation names exactly
// one Heap root, and an opaque reference is the only atom here that needs
// conservative widening.
func TestPlacementAtomClassificationKeepsBootAllocationAndOpaqueDistinct(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	if fixture.heap.BootCount() == 0 {
		t.Skip("fixture has no detached Boot root")
	}
	if len(fixture.allocations) == 0 {
		t.Fatal("fixture has no allocation root")
	}

	bootID, bootOK := fixture.heap.BootIDAt(0)
	bootAtom, bootAtomOK := fixture.values.BootID(bootID)
	allocationAtom, allocationOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	opaqueAtom, opaqueOK := fixture.values.OpaqueReference(valuedomain.ReferenceTable)
	if !bootOK || !bootAtomOK || !allocationOK || !opaqueOK {
		t.Fatalf("atoms boot=%t/%t allocation=%t opaque=%t", bootOK, bootAtomOK, allocationOK, opaqueOK)
	}

	allocation := placementdomain.AtomClassification{Class: placementdomain.AtomClassAllocation, Key: fixture.allocations[0], Role: materialization.Recent}
	cases := []struct {
		name  string
		atom  valuedomain.Atom
		class placementdomain.AtomClass
		want  placementdomain.AtomClassification
	}{
		{name: "boot", atom: bootAtom, class: placementdomain.AtomClassBoot},
		{name: "allocation", atom: allocationAtom, class: placementdomain.AtomClassAllocation, want: allocation},
		{name: "opaque", atom: opaqueAtom, class: placementdomain.AtomClassOpaque},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, gotOK := placementdomain.ClassifyAtom(fixture.values, test.atom)
			if !gotOK || !got.Valid() || got.Class != test.class {
				t.Fatalf("classification = %#v/%t, want class %v", got, gotOK, test.class)
			}
			if test.class == placementdomain.AtomClassAllocation && got != test.want {
				t.Fatalf("allocation classification = %#v, want %#v", got, test.want)
			}
		})
	}

	if allocations := testing.AllocsPerRun(100, func() {
		first, firstOK := placementdomain.ClassifyAtom(fixture.values, bootAtom)
		second, secondOK := placementdomain.ClassifyAtom(fixture.values, allocationAtom)
		third, thirdOK := placementdomain.ClassifyAtom(fixture.values, opaqueAtom)
		placementAtomClassificationSink = first
		placementAtomClassificationOK = firstOK && secondOK && thirdOK && second.Class == placementdomain.AtomClassAllocation && third.Class == placementdomain.AtomClassOpaque
	}); allocations != 0 {
		t.Fatalf("atom classification allocations = %v, want zero", allocations)
	}
	if !placementAtomClassificationOK || placementAtomClassificationSink.Class != placementdomain.AtomClassBoot {
		t.Fatal("classification benchmark sink was not populated")
	}
}
