package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type PathRefinementsSnapshot struct {
	Top         bool
	Refinements map[pathdom.PathKey]product.Value
}

// PathRefinementsSnapshot returns finite path refinements unless the path lane
// is top. When Top is true, Refinements is empty and callers must not
// manufacture finite facts from it.
func (s State) PathRefinementsSnapshot() PathRefinementsSnapshot {
	snapshot := s.pathEvidence.PathRefinementsSnapshot()
	return PathRefinementsSnapshot{
		Top:         snapshot.Top,
		Refinements: snapshot.Refinements,
	}
}

type PathStaticMembersSnapshot struct {
	Bottom  bool
	Top     bool
	Members map[pathdom.PathKey]product.Value
}

// PathStaticMembersSnapshot returns finite must-static-member facts. Bottom is
// explicit; Top means the reachable must lane contains no finite facts.
func (s State) PathStaticMembersSnapshot() PathStaticMembersSnapshot {
	snapshot := s.pathEvidence.PathStaticMembersSnapshot()
	return PathStaticMembersSnapshot{
		Bottom:  snapshot.Bottom,
		Top:     snapshot.Top,
		Members: snapshot.Members,
	}
}

type DynamicIndexFactsSnapshot struct {
	Top   bool
	Facts map[dynamicindex.Key]dynamicindex.Fact
}

// DynamicIndexFactsSnapshot returns finite dynamic-index facts unless the lane
// is top. When Top is true, Facts is empty.
func (s State) DynamicIndexFactsSnapshot() DynamicIndexFactsSnapshot {
	if s.dynamicIndexTop {
		return DynamicIndexFactsSnapshot{Top: true}
	}
	return DynamicIndexFactsSnapshot{Facts: dynamicindex.CloneMap(s.dynamicIndex)}
}

type BranchProofsSnapshot struct {
	Bottom bool
	Top    bool
	Proofs []pathevidence.BranchProof
}

// BranchProofsSnapshot returns finite must branch proofs in stable order.
// Bottom is explicit; Top means the reachable must lane contains no proofs.
func (s State) BranchProofsSnapshot() BranchProofsSnapshot {
	snapshot := s.pathEvidence.BranchProofsSnapshot()
	return BranchProofsSnapshot{
		Bottom: snapshot.Bottom,
		Top:    snapshot.Top,
		Proofs: snapshot.Proofs,
	}
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
	if s.effectDeltasTop {
		return EffectDeltasSnapshot{Top: true}
	}
	return EffectDeltasSnapshot{Deltas: effectdelta.CloneMap(s.effectDeltas)}
}
