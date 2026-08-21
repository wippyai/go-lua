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
		{lifecycle.SubjectLivenessUnknown, placement.Unknown},
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
	// DiesBefore is the least demand, Live requires a surviving owned root,
	// and Unknown is the conservative top. The consumer must not reverse this
	// ordering while projecting the neutral answer.
	if !placement.LessOrEq(placement.Stack, placement.OwnedHeap) || !placement.LessOrEq(placement.OwnedHeap, placement.Unknown) {
		t.Fatal("placement order no longer has Stack <= OwnedHeap <= Unknown")
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
