package state

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type PathRefinementsSnapshot struct {
	Top         bool
	Refinements map[pathdom.PathKey]product.Value
}

// PathRefinementsSnapshot returns finite path refinements unless the path lane
// is top. When Top is true, Refinements is empty and callers must not
// manufacture finite facts from it.
func (s State) PathRefinementsSnapshot() PathRefinementsSnapshot {
	if s.pathsTop {
		return PathRefinementsSnapshot{Top: true}
	}
	return PathRefinementsSnapshot{Refinements: clonePathMap(s.paths)}
}

type PathStaticMembersSnapshot struct {
	Bottom  bool
	Top     bool
	Members map[pathdom.PathKey]product.Value
}

// PathStaticMembersSnapshot returns finite must-static-member facts. Bottom is
// explicit; Top means the reachable must lane contains no finite facts.
func (s State) PathStaticMembersSnapshot() PathStaticMembersSnapshot {
	if s.pathStaticMembersBottom {
		return PathStaticMembersSnapshot{Bottom: true}
	}
	members := clonePathMap(s.pathStaticMembers)
	return PathStaticMembersSnapshot{
		Top:     len(members) == 0,
		Members: members,
	}
}

type DynamicIndexFactsSnapshot struct {
	Top   bool
	Facts map[DynamicIndexKey]DynamicIndexFact
}

// DynamicIndexFactsSnapshot returns finite dynamic-index facts unless the lane
// is top. When Top is true, Facts is empty.
func (s State) DynamicIndexFactsSnapshot() DynamicIndexFactsSnapshot {
	if s.dynamicIndexTop {
		return DynamicIndexFactsSnapshot{Top: true}
	}
	return DynamicIndexFactsSnapshot{Facts: cloneDynamicIndexMap(s.dynamicIndex)}
}

type BranchProofsSnapshot struct {
	Bottom bool
	Top    bool
	Proofs []BranchProof
}

// BranchProofsSnapshot returns finite must branch proofs in stable order.
// Bottom is explicit; Top means the reachable must lane contains no proofs.
func (s State) BranchProofsSnapshot() BranchProofsSnapshot {
	if s.branchProofsBottom {
		return BranchProofsSnapshot{Bottom: true}
	}
	proofs := branchProofsFromSet(s.branchProofs)
	return BranchProofsSnapshot{
		Top:    len(proofs) == 0,
		Proofs: proofs,
	}
}

type ChannelSelectFactsSnapshot struct {
	Bottom bool
	Top    bool
	Facts  []ChannelSelectFact
}

// ChannelSelectFactsSnapshot returns finite must channel-select facts in stable
// order. Bottom is explicit; Top means the reachable must lane contains no
// facts.
func (s State) ChannelSelectFactsSnapshot() ChannelSelectFactsSnapshot {
	if s.channelSelectBottom {
		return ChannelSelectFactsSnapshot{Bottom: true}
	}
	facts := channelSelectFactsFromSet(s.channelSelect)
	return ChannelSelectFactsSnapshot{
		Top:   len(facts) == 0,
		Facts: facts,
	}
}

type EffectDeltasSnapshot struct {
	Top    bool
	Deltas map[EffectDeltaKey]EffectDelta
}

// EffectDeltasSnapshot returns finite effect deltas unless the lane is top.
// When Top is true, Deltas is empty.
func (s State) EffectDeltasSnapshot() EffectDeltasSnapshot {
	if s.effectDeltasTop {
		return EffectDeltasSnapshot{Top: true}
	}
	return EffectDeltasSnapshot{Deltas: cloneEffectDeltaMap(s.effectDeltas)}
}

func branchProofsFromSet(in map[BranchProof]struct{}) []BranchProof {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchProof, 0, len(in))
	for proof := range in {
		out = append(out, proof)
	}
	sort.Slice(out, func(i, j int) bool {
		return branchProofLess(out[i], out[j])
	})
	return out
}

func branchProofLess(a, b BranchProof) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Other != b.Other {
		return a.Other < b.Other
	}
	return a.Presence.String() < b.Presence.String()
}

func channelSelectFactsFromSet(in map[ChannelSelectFact]struct{}) []ChannelSelectFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChannelSelectFact, 0, len(in))
	for fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		return channelSelectFactLess(out[i], out[j])
	})
	return out
}

func channelSelectFactLess(a, b ChannelSelectFact) bool {
	if a.Select != b.Select {
		return a.Select < b.Select
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Result != b.Result {
		return a.Result < b.Result
	}
	if a.Case != b.Case {
		return a.Case < b.Case
	}
	return a.Index < b.Index
}
