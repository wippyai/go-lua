package composite

import "testing"

// TestMountPhaseRejectsAnAbsentArtifactView states the phase's admission: the
// Link and its neutral artifact view are the mount phase's whole input, so a
// record carrying neither is rejected before any axis is asked to seal.
func TestMountPhaseRejectsAnAbsentArtifactView(t *testing.T) {
	mounted, failure := MountLink(LinkInputs{})
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
	sealRegistry()
	if registry.sealed == nil {
		t.Fatalf("declaration table did not seal: %v", registry.failure)
	}
	declared := 0
	for position, entry := range registry.axes {
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
		if adopted != (registry.axisAdopters[slot] != nil) {
			t.Fatalf("axis %q is adopted at a slot the mount path does not hold", entry.Key())
		}
	}
	if declared == 0 {
		t.Fatalf("no axis seals its own authority; the mount phase is unreachable")
	}
}

// TestAdoptionRejectsADeclaredMountThatSealedNothing states that adoption is
// not optional for a declared mount: an axis that owns its authority and
// produced none leaves the record incomplete, and the phase says so at that
// axis rather than handing a half-filled record to the binding transaction.
func TestAdoptionRejectsADeclaredMountThatSealedNothing(t *testing.T) {
	sealRegistry()
	if registry.sealed == nil {
		t.Fatalf("declaration table did not seal: %v", registry.failure)
	}
	supplied := newAxisCells(registry.axes)
	inputs, failedAxis, ok := LinkInputs{}.adopt(supplied)
	if ok {
		t.Fatalf("adoption admitted a phase in which every declared mount sealed nothing")
	}
	blamed, blamedOK := axisAtSlot(int(failedAxis))
	if !blamedOK || !blamed.MountDeclared() {
		t.Fatalf("adoption blamed axis %v, which declares no mount", failedAxis)
	}
	if inputs.ValueSchema != nil {
		t.Fatalf("rejected adoption published an authority")
	}
}
