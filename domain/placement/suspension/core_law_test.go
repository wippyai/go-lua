package suspension

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/domain/placement"
)

func TestPlacementForStateIsClosedAndConservative(t *testing.T) {
	cases := []struct {
		state lifecycle.SubjectLivenessState
		want  placement.Placement
	}{
		{lifecycle.SubjectLivenessDiesBefore, placement.Stack},
		{lifecycle.SubjectLivenessLive, placement.OwnedHeap},
		{lifecycle.SubjectLivenessUnknown, placement.OwnedHeap},
	}
	for _, item := range cases {
		got, ok := PlacementForState(item.state)
		if !ok || got != item.want {
			t.Fatalf("PlacementForState(%v) = %v/%t, want %v/true", item.state, got, ok, item.want)
		}
	}
	for raw := 0; raw < 256; raw++ {
		state := lifecycle.SubjectLivenessState(raw)
		if state.Valid() {
			continue
		}
		if got, ok := PlacementForState(state); ok || got != placement.Bottom {
			t.Fatalf("invalid state %d = %v/%t, want Bottom/false", raw, got, ok)
		}
	}
}

func TestPlacementForStatePreservesTheLivenessOrder(t *testing.T) {
	// DiesBefore is the least demand. Live and Unknown both require a surviving
	// owned root: liveness uncertainty carries no sharing evidence and therefore
	// cannot select Placement Top.
	if !placement.LessOrEq(placement.Stack, placement.OwnedHeap) {
		t.Fatal("placement order no longer has Stack <= OwnedHeap")
	}
	for _, state := range []lifecycle.SubjectLivenessState{
		lifecycle.SubjectLivenessDiesBefore,
		lifecycle.SubjectLivenessLive,
		lifecycle.SubjectLivenessUnknown,
	} {
		got, ok := PlacementForState(state)
		if !ok || !placement.LessOrEq(placement.Stack, got) {
			t.Fatalf("state %v projected below the seeded Stack placement: %v/%t", state, got, ok)
		}
	}
}

func TestUnknownSuspensionCannotPoisonAuthenticatedSharing(t *testing.T) {
	suspension, suspensionOK := PlacementForState(lifecycle.SubjectLivenessUnknown)
	joined, joinedOK := placement.JoinChecked(suspension, placement.SharedHeap)
	if !suspensionOK || !joinedOK || joined != placement.SharedHeap {
		t.Fatalf("unknown suspension joined with authenticated sharing = %v/%t, want SharedHeap", joined, joinedOK)
	}
}
