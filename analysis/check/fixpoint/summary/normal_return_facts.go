package summary

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

// NormalReturnFacts is the summary-local payload lane for facts that hold on a
// normal return and can cross function boundaries through placeholder paths.
type NormalReturnFacts struct {
	PathRefinements   []PathValueFact
	PathStaticMembers []PathStaticMemberFact
	DynamicIndexFacts []DynamicIndexFact
	BranchProofs      []BranchProof
	ChannelSelects    []ChannelSelectFact
	EffectDeltas      []EffectDelta
}

// PathValueFact records a pointwise placeholder-path value refinement.
type PathValueFact struct {
	Path  pathdom.Path
	Value product.Value
}

// PathStaticMemberFact records a must static-member fact for a placeholder path.
type PathStaticMemberFact struct {
	Path  pathdom.Path
	Value product.Value
}

// DynamicIndexFact records a pointwise dynamic index fact for a placeholder table.
type DynamicIndexFact struct {
	Table pathdom.Path
	Site  dynamicindex.Site
	Value dynamicindex.Fact
}

// BranchProofKind classifies a summary-local branch proof.
type BranchProofKind uint8

const (
	BranchProofPathPresence BranchProofKind = iota + 1
	BranchProofPathEqual
	BranchProofPathNotEqual
)

// BranchProof records a must branch proof over placeholder paths.
type BranchProof struct {
	Kind     BranchProofKind
	Path     pathdom.Path
	Presence presence.Value
	Other    pathdom.Path
}

// ChannelSelectFactKind classifies a summary-local channel select fact.
type ChannelSelectFactKind uint8

const (
	ChannelSelectFactSelect ChannelSelectFactKind = iota + 1
	ChannelSelectFactReceive
	ChannelSelectFactCase
)

// ChannelSelectFact records a must channel-select fact with stable caller-provided IDs.
type ChannelSelectFact struct {
	Select string
	Kind   ChannelSelectFactKind
	Result pathdom.Path
	Case   pathdom.Path
	Index  int
}

// EffectDelta records a pointwise effect delta for a placeholder target path.
type EffectDelta struct {
	Target pathdom.Path
	Site   effectdelta.Site
	Kind   effectdelta.Kind
	Value  effectdelta.Value
}

func normalizeNormalReturnFacts(reg *axis.Registry, in NormalReturnFacts) NormalReturnFacts {
	out := NormalReturnFacts{
		PathRefinements:   normalizePathValueFacts(reg, in.PathRefinements),
		PathStaticMembers: normalizePathStaticMemberFacts(reg, in.PathStaticMembers),
		DynamicIndexFacts: normalizeDynamicIndexFacts(reg, in.DynamicIndexFacts),
		BranchProofs:      normalizeBranchProofs(in.BranchProofs),
		ChannelSelects:    normalizeChannelSelectFacts(in.ChannelSelects),
		EffectDeltas:      normalizeEffectDeltas(reg, in.EffectDeltas),
	}
	if normalReturnFactsEmpty(out) {
		return NormalReturnFacts{}
	}
	return out
}

func cloneNormalReturnFacts(in NormalReturnFacts) NormalReturnFacts {
	if normalReturnFactsEmpty(in) {
		return NormalReturnFacts{}
	}
	return NormalReturnFacts{
		PathRefinements:   clonePathValueFacts(in.PathRefinements),
		PathStaticMembers: clonePathStaticMemberFacts(in.PathStaticMembers),
		DynamicIndexFacts: cloneDynamicIndexFacts(in.DynamicIndexFacts),
		BranchProofs:      cloneBranchProofs(in.BranchProofs),
		ChannelSelects:    cloneChannelSelectFacts(in.ChannelSelects),
		EffectDeltas:      cloneEffectDeltas(in.EffectDeltas),
	}
}

func normalReturnFactsEqual(reg *axis.Registry, a, b NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return pathValueFactsEqual(reg, a.PathRefinements, b.PathRefinements) &&
		pathStaticMemberFactsEqual(reg, a.PathStaticMembers, b.PathStaticMembers) &&
		dynamicIndexFactsEqual(reg, a.DynamicIndexFacts, b.DynamicIndexFacts) &&
		branchProofsEqual(a.BranchProofs, b.BranchProofs) &&
		channelSelectFactsEqual(a.ChannelSelects, b.ChannelSelects) &&
		effectDeltasEqual(reg, a.EffectDeltas, b.EffectDeltas)
}

func normalReturnFactsLessOrEq(reg *axis.Registry, a, b NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return pathValueFactsLessOrEq(reg, a.PathRefinements, b.PathRefinements) &&
		pathStaticMemberFactsLessOrEq(reg, a.PathStaticMembers, b.PathStaticMembers) &&
		dynamicIndexFactsLessOrEq(reg, a.DynamicIndexFacts, b.DynamicIndexFacts) &&
		branchProofsLessOrEq(a.BranchProofs, b.BranchProofs) &&
		channelSelectFactsLessOrEq(a.ChannelSelects, b.ChannelSelects) &&
		effectDeltasLessOrEq(reg, a.EffectDeltas, b.EffectDeltas)
}

func joinNormalReturnFacts(reg *axis.Registry, a, b NormalReturnFacts) NormalReturnFacts {
	return normalizeNormalReturnFacts(reg, NormalReturnFacts{
		PathRefinements:   joinPathValueFacts(reg, a.PathRefinements, b.PathRefinements),
		PathStaticMembers: joinPathStaticMemberFacts(reg, a.PathStaticMembers, b.PathStaticMembers),
		DynamicIndexFacts: joinDynamicIndexFacts(reg, a.DynamicIndexFacts, b.DynamicIndexFacts),
		BranchProofs:      joinBranchProofs(a.BranchProofs, b.BranchProofs),
		ChannelSelects:    joinChannelSelectFacts(a.ChannelSelects, b.ChannelSelects),
		EffectDeltas:      joinEffectDeltas(reg, a.EffectDeltas, b.EffectDeltas),
	})
}

func widenNormalReturnFacts(reg *axis.Registry, prev, next NormalReturnFacts) NormalReturnFacts {
	return normalizeNormalReturnFacts(reg, NormalReturnFacts{
		PathRefinements:   widenPathValueFacts(reg, prev.PathRefinements, next.PathRefinements),
		PathStaticMembers: widenPathStaticMemberFacts(reg, prev.PathStaticMembers, next.PathStaticMembers),
		DynamicIndexFacts: widenDynamicIndexFacts(reg, prev.DynamicIndexFacts, next.DynamicIndexFacts),
		BranchProofs:      joinBranchProofs(prev.BranchProofs, next.BranchProofs),
		ChannelSelects:    joinChannelSelectFacts(prev.ChannelSelects, next.ChannelSelects),
		EffectDeltas:      widenEffectDeltas(reg, prev.EffectDeltas, next.EffectDeltas),
	})
}

func normalReturnFactsEmpty(facts NormalReturnFacts) bool {
	return len(facts.PathRefinements) == 0 &&
		len(facts.PathStaticMembers) == 0 &&
		len(facts.DynamicIndexFacts) == 0 &&
		len(facts.BranchProofs) == 0 &&
		len(facts.ChannelSelects) == 0 &&
		len(facts.EffectDeltas) == 0
}
