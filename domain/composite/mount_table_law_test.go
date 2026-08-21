package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis"
)

// TestMountPhaseRejectsAnAbsentArtifactView states the phase's admission: the
// Link and its neutral artifact view are the mount phase's whole input, so a
// record carrying neither is rejected before any axis is asked to seal.
func TestMountPhaseRejectsAnAbsentArtifactView(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	mounted, failure := MountLink(compilation, LinkInputs{})
	if !failure.Available() || failure.Stage != MountStageInput {
		t.Fatalf("mount phase admitted an empty record: %v", failure)
	}
	if mounted.ValueSchema != nil || mounted.Source != nil {
		t.Fatalf("rejected mount phase published a partially mounted record")
	}
	if _, ok := MountRejection[int](failure); ok {
		t.Fatalf("a phase rejection with no axis carried domain evidence")
	}
}

// TestEveryDeclaredMountIsAdopted is the coverage law binding the two halves of
// the phase: an axis that seals its own authority must have a typed adopter in
// the Link input record, and the record must adopt nothing no axis sealed.
// A domain moved onto the declared path without its adopter would mount an
// authority the binding transaction never receives, and this states that.
func TestEveryDeclaredMountIsAdopted(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		t.Fatal("declaration table did not seal")
	}
	declared := 0
	for position, entry := range state.axes {
		slot := position + 1
		_, adopted := axisAdopterFor(entry.Key())
		if entry.MountDeclared() {
			declared++
			if !adopted {
				t.Fatalf("axis %q seals its own authority with no adopter in the Link input record", entry.Key())
			}
		} else if adopted {
			t.Fatalf("axis %q is adopted but declares no mount", entry.Key())
		}
		if adopted != (state.axisAdopters[slot] != nil) {
			t.Fatalf("axis %q is adopted at a slot the mount path does not hold", entry.Key())
		}
	}
	if declared == 0 {
		t.Fatalf("no axis seals its own authority; the mount phase is unreachable")
	}
}

// TestPlacementAdopterKeepsTheHeapOwnerFence states the exact join made while
// adopting Placement's mounted authority. Placement projects Heap's dense
// coordinate space, so a placement schema from another Heap must not enter the
// Link input record even when its payload is otherwise well typed.
func TestPlacementAdopterKeepsTheHeapOwnerFence(t *testing.T) {
	owned := mountedRecord(t, "placement-adopter-owned", "local root = {}; return root")
	foreign := mountedRecord(t, "placement-adopter-foreign", "local root = {value = 1}; return root")
	if !owned.PlacementSchema.Valid() || !owned.HeapSchema.Valid() || !foreign.HeapSchema.Valid() {
		t.Fatal("placement adopter fixture did not seal its Heap and Placement authorities")
	}
	if owned.HeapSchema.ContentID() == foreign.HeapSchema.ContentID() {
		t.Fatal("placement adopter fixture did not produce distinct Heap authorities")
	}
	adopter, adopterOK := axisAdopterFor(axisKeyPlacement)
	if !adopterOK || adopter == nil {
		t.Fatal("Placement axis has no typed adopter")
	}

	matching, matched := adopter(LinkInputs{HeapSchema: owned.HeapSchema}, axis.NewCell(owned.PlacementSchema))
	if !matched || matching.PlacementSchema.ContentID() != owned.PlacementSchema.ContentID() {
		t.Fatal("Placement adopter refused the Placement schema projected from the exact Heap")
	}
	foreignInput, crossed := adopter(LinkInputs{HeapSchema: foreign.HeapSchema}, axis.NewCell(owned.PlacementSchema))
	if crossed || foreignInput.PlacementSchema.Valid() {
		t.Fatal("Placement adopter crossed a foreign Heap owner fence")
	}
}

// TestAdoptionRejectsADeclaredMountThatSealedNothing states that adoption is
// not optional for a declared mount: an axis that owns its authority and
// produced none leaves the record incomplete, and the phase says so at that
// axis rather than handing a half-filled record to the binding transaction.
func TestAdoptionRejectsADeclaredMountThatSealedNothing(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		t.Fatal("declaration table did not seal")
	}
	supplied := newAxisCells(state.axes)
	inputs, failedAxis, ok := LinkInputs{}.adopt(state, supplied)
	if ok {
		t.Fatalf("adoption admitted a phase in which every declared mount sealed nothing")
	}
	blamed, blamedOK := axisAtSlot(state, int(failedAxis))
	if !blamedOK || !blamed.MountDeclared() {
		t.Fatalf("adoption blamed axis %v, which declares no mount", failedAxis)
	}
	if inputs.ValueSchema != nil {
		t.Fatalf("rejected adoption published an authority")
	}
}
