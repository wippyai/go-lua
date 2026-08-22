package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
)

// TestPackAxisMountRejectionRendersItsReason states the observability law for
// a rejected pack-axis mount: the erased reason cell recovers at pack's own
// evidence type, so a pack-axis rejection is never reported as a bare axis
// identity with its cause discarded. valuedomain.SealFailure is the sibling
// law this mirrors for the value axis.
func TestPackAxisMountRejectionRendersItsReason(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("sealed compilation unavailable")
	}
	packAxis := DiagnosticAxisForKey(compilation, axisKeyPack)
	if packAxis == DiagnosticAxisUnknown {
		t.Fatal("the pack axis classifies as unknown")
	}
	for _, rejection := range []packowner.MountRejection{packowner.MountRejectionInput, packowner.MountRejectionSeal} {
		failure := MountFailure{Stage: MountStageAxis, Axis: packAxis, reason: axis.NewCell(rejection)}
		if !failure.Available() {
			t.Fatalf("%s: the mount failure reports unavailable", rejection)
		}
		if got, ok := MountRejection[packowner.MountRejection](failure); !ok || got != rejection {
			t.Fatalf("%s: recovered pack rejection %v/%t", rejection, got, ok)
		}
		if got, ok := MountRejection[int](failure); ok {
			t.Fatalf("%s: a foreign type recovered a pack rejection: %v", rejection, got)
		}
	}
	// A rejection from another axis carries no pack evidence: recovering it
	// at pack's own type must report absent rather than a foreign guess.
	foreign := MountFailure{Stage: MountStageAxis, Axis: packAxis, reason: axis.NewCell(42)}
	if _, ok := MountRejection[packowner.MountRejection](foreign); ok {
		t.Fatal("a foreign reason type recovered as a pack rejection")
	}
}
