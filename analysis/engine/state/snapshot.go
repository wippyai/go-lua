package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

type ValuesSnapshot struct {
	Top    bool
	Values map[key.Value]product.Value
}

// ValuesSnapshot returns finite value slots unless the value lane is top. When
// Top is true, Values is empty.
func (s State) ValuesSnapshot() ValuesSnapshot {
	if !s.laneEnabled(laneValuesBit) {
		return ValuesSnapshot{}
	}
	if s.values.top {
		return ValuesSnapshot{Top: true}
	}
	return ValuesSnapshot{Values: s.values.cloneValues()}
}

type DynamicIndexFactsSnapshot struct {
	Top   bool
	Facts map[dynamicindex.Key]dynamicindex.Fact
}

// DynamicIndexFactsSnapshot returns finite dynamic-index facts unless the lane
// is top. When Top is true, Facts is empty.
func (s State) DynamicIndexFactsSnapshot() DynamicIndexFactsSnapshot {
	if !s.laneEnabled(laneDynamicIndexBit) {
		return DynamicIndexFactsSnapshot{}
	}
	return dynamicIndexFactsSnapshot(s.dynamicIndex)
}

func dynamicIndexFactsSnapshot(lane dynamicIndexLane) DynamicIndexFactsSnapshot {
	if lane.top {
		return DynamicIndexFactsSnapshot{Top: true}
	}
	return DynamicIndexFactsSnapshot{Facts: lane.cloneValues()}
}

type ChannelSelectFactsSnapshot struct {
	Bottom bool
	Top    bool
	Facts  []channelselectfact.Fact
}

// ChannelSelectFactsSnapshot returns finite must channel-select facts in stable
// order. Bottom is explicit; Top means the reachable must lane contains no
// facts.
func (s State) ChannelSelectFactsSnapshot() ChannelSelectFactsSnapshot {
	if !s.laneEnabled(laneChannelSelectBit) {
		return ChannelSelectFactsSnapshot{Bottom: true}
	}
	return channelSelectFactsSnapshot(s.channelSelect)
}

func channelSelectFactsSnapshot(lane channelselectfact.Lane) ChannelSelectFactsSnapshot {
	snapshot := lane.Snapshot()
	return ChannelSelectFactsSnapshot{
		Bottom: snapshot.Bottom,
		Top:    snapshot.Top,
		Facts:  snapshot.Facts,
	}
}

type EffectDeltasSnapshot struct {
	Top    bool
	Deltas map[effectdelta.Key]effectdelta.Value
}

// EffectDeltasSnapshot returns finite effect deltas unless the lane is top.
// When Top is true, Deltas is empty.
func (s State) EffectDeltasSnapshot() EffectDeltasSnapshot {
	if !s.laneEnabled(laneEffectDeltasBit) {
		return EffectDeltasSnapshot{}
	}
	return effectDeltasSnapshot(s.effectDeltas)
}

func effectDeltasSnapshot(lane effectDeltaLane) EffectDeltasSnapshot {
	if lane.top {
		return EffectDeltasSnapshot{Top: true}
	}
	return EffectDeltasSnapshot{Deltas: lane.cloneValues()}
}

type HeapTableObjectsSnapshot struct {
	Top     bool
	Objects map[identity.ID]heapidentity.TableObject
}

// HeapTableObjectsSnapshot returns finite identity-keyed heap table objects
// unless the lane is top. When Top is true, Objects is empty.
func (s State) HeapTableObjectsSnapshot() HeapTableObjectsSnapshot {
	if !s.laneEnabled(laneHeapTableIdentityBit) {
		return HeapTableObjectsSnapshot{}
	}
	return heapTableObjectsSnapshot(s.heapTableIdentity)
}

func heapTableObjectsSnapshot(lane heapTableIdentityLane) HeapTableObjectsSnapshot {
	// This legacy concrete snapshot cannot spell an unresolved relational key.
	// Preserve soundness as Top; publication callers that require exact finite
	// evidence use checkedHeapTableObjectsSnapshot and receive the typed error.
	snapshot, _ := checkedHeapTableObjectsSnapshot(lane)
	return snapshot
}

// checkedHeapTableObjectsSnapshot is the single concrete publication law for
// the relational heap lane. Formal and allocation terms are exact inside the
// solver, but they cannot cross the concrete CallOutcome boundary before the
// owning substitution transaction has materialized them.
func checkedHeapTableObjectsSnapshot(lane heapTableIdentityLane) (HeapTableObjectsSnapshot, error) {
	if lane.top {
		return HeapTableObjectsSnapshot{Top: true}, nil
	}
	objects := make(map[identity.ID]heapidentity.TableObject, len(lane.values))
	for term, object := range lane.values {
		id, concrete := term.Concrete()
		if !concrete {
			return HeapTableObjectsSnapshot{Top: true}, fmt.Errorf("state: unresolved relational heap identity crossed concrete snapshot boundary")
		}
		objects[id] = heapidentity.CloneObject(object)
	}
	return HeapTableObjectsSnapshot{Objects: objects}, nil
}

type PlacementsSnapshot struct {
	Top        bool
	Placements map[identity.ID]placement.Value
}

// PlacementsSnapshot returns finite identity-keyed allocation placements unless
// the lane is top. Missing entries are not stack-safe proofs; they mean no
// finite placement fact is available for that identity.
func (s State) PlacementsSnapshot() PlacementsSnapshot {
	if !s.laneEnabled(lanePlacementBit) {
		return PlacementsSnapshot{}
	}
	// See heapTableObjectsSnapshot: unresolved relational keys conservatively
	// observe as Top, while exact publication uses the checked law below.
	snapshot, _ := checkedPlacementsSnapshot(s.placement)
	return snapshot
}

// checkedPlacementsSnapshot shares the same concrete publication fence as
// heap objects. Keeping it beside the State snapshot prevents factor views
// from growing a second placement interpretation.
func checkedPlacementsSnapshot(lane placementLane) (PlacementsSnapshot, error) {
	if lane.top {
		return PlacementsSnapshot{Top: true}, nil
	}
	placements := make(map[identity.ID]placement.Value, len(lane.values))
	for term, value := range lane.values {
		id, concrete := term.Concrete()
		if !concrete {
			return PlacementsSnapshot{Top: true}, fmt.Errorf("state: unresolved relational placement identity crossed concrete snapshot boundary")
		}
		placements[id] = value
	}
	return PlacementsSnapshot{Placements: placements}, nil
}
