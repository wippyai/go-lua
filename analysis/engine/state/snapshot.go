package state

import (
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
	if s.dynamicIndex.top {
		return DynamicIndexFactsSnapshot{Top: true}
	}
	return DynamicIndexFactsSnapshot{Facts: s.dynamicIndex.cloneValues()}
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
	snapshot := s.channelSelect.Snapshot()
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
	if s.effectDeltas.top {
		return EffectDeltasSnapshot{Top: true}
	}
	return EffectDeltasSnapshot{Deltas: s.effectDeltas.cloneValues()}
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
	if s.heapTableIdentity.top {
		return HeapTableObjectsSnapshot{Top: true}
	}
	return HeapTableObjectsSnapshot{Objects: heapidentity.CloneMap(s.heapTableIdentity.values)}
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
	if s.placement.top {
		return PlacementsSnapshot{Top: true}
	}
	return PlacementsSnapshot{Placements: s.placement.cloneValues()}
}
