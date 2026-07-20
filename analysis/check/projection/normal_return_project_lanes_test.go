package projection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestNormalReturnProjectLaneRegistryContainsSyntaxSupplementsOnly(t *testing.T) {
	want := map[callboundary.NormalReturnFactLaneID]bool{
		callboundary.LanePathInvalidations: true, callboundary.LanePersistentPathWrites: true,
		callboundary.LanePathStaticMemberDeltas: true, callboundary.LaneDynamicIndexFacts: true,
		callboundary.LaneDynamicValueKeys: true, callboundary.LaneStoreRelations: true,
		callboundary.LaneLifecycleFacts: true,
	}
	for _, lane := range normalReturnProjectLanes {
		if !want[lane.lane] || lane.project == nil {
			t.Fatalf("unexpected normal-return supplement %#v", lane)
		}
		delete(want, lane.lane)
	}
	if len(want) != 0 {
		t.Fatalf("missing normal-return supplements: %#v", want)
	}
}
