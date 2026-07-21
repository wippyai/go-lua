package callboundary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

// TestNormalReturnFactsFromProjectedStateTransportsNumCeilAcrossCallBoundary
// pins register item #1682: a callee-proven numeric upper bound never crosses
// a call boundary.
// NormalReturnFacts carries a full NumFloors lane end to end (LaneNumFloors in
// normal_return_lanes.go, its algebra in normal_return_num_floor_algebra.go,
// and projectStateNumFloors in projected_state_facts.go), but
// NormalReturnFactLaneID has no NumCeils counterpart anywhere, even though
// journal #1761 unified NumFloor
// and NumCeil into one direction-parametric coordinate family inside
// analysis/engine/state (see numCeilsLaneSpec/numFloorsLaneSpec and
// state/numbound). That unification stayed intraprocedural: it never reached
// the call-boundary summary surface this package owns.
//
// A sound call-boundary summary must carry strictly more information when the
// callee's exit State proves strictly more: adding one proven fact to the
// source State must add at least one fact somewhere in the projection. This
// test asserts exactly that correct behavior for a proven NumCeil and fails
// today, because no lane transports it. It must be updated once a NumCeils
// lane exists and actually carries the fact.
func TestNormalReturnFactsFromProjectedStateTransportsNumCeilAcrossCallBoundary(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	root := pathdom.Path{Root: "arg", Symbol: symbol.ID(902), Version: 1}
	rootKey := keys.FromPath(root)
	rootState := stateKey(t, root)
	id := identity.ID{Kind: "lua.table", Site: "num-ceil-boundary-gap", Index: 1}
	idValue := identityvalue.Present(reg, id)
	roots := state.BoundaryRoots{{Slot: key.SymbolValue(root.Symbol), Path: rootKey, Value: idValue}}

	project := func(withCeil bool) NormalReturnFacts {
		world := state.Domain(reg).Bottom().WriteValue(reg, key.SymbolValue(root.Symbol), idValue)
		if withCeil {
			world = world.WriteNumCeil(keys, rootState, 9)
			if ceil, ok := world.ReadNumCeil(keys, rootState); !ok || ceil != 9 {
				t.Fatalf("sanity check failed: source State does not carry the proven ceil, got (%d,%v)", ceil, ok)
			}
		}
		artifact, err := state.ProjectBoundary(reg, keys, world, roots)
		if err != nil {
			t.Fatal(err)
		}
		projected, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
		if err != nil {
			t.Fatal(err)
		}
		facts, err := NormalReturnFactsFromProjectedState(reg, keys, projected, projectedRoots, 1)
		if err != nil {
			t.Fatal(err)
		}
		return facts
	}

	without := project(false)
	with := project(true)

	totalWithout, totalWith := 0, 0
	for _, lane := range NormalReturnFactLanes() {
		totalWithout += lane.Len(without)
		totalWith += lane.Len(with)
	}
	if totalWith <= totalWithout {
		t.Fatalf("projected fact count with proven ceil=%d, without=%d; want strictly more facts with the ceil present, but no lane transports NumCeil across the call boundary", totalWith, totalWithout)
	}
}
