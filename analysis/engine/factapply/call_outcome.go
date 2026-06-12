package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// CallOutcome is the generic payload produced at a call boundary. It carries
// return-slot values plus normal-return facts expressed over placeholder paths
// such as $0 and $1. Fact application rebases those paths at the caller.
type CallOutcome struct {
	Results []CallResult

	PathRefinements   []CallPathRefinement
	PathStaticMembers []CallPathStaticMember
	DynamicIndexFacts []CallDynamicIndexFact
	BranchProofs      []CallBranchProof
	ChannelSelects    []CallChannelSelectFact
	EffectDeltas      []CallEffectDelta
}

// CallPathRefinement records a normal-return value constraint for a
// placeholder-rooted path.
type CallPathRefinement struct {
	Path  pathdom.Path
	Value product.Value
}

// CallPathStaticMember records a normal-return static-member fact for a
// placeholder-rooted path.
type CallPathStaticMember struct {
	Path  pathdom.Path
	Value product.Value
}

// CallDynamicIndexAdmission summarizes whether a dynamic index write was
// admitted at the callee boundary.
type CallDynamicIndexAdmission uint8

const (
	CallDynamicIndexAdmissionBottom CallDynamicIndexAdmission = iota
	CallDynamicIndexAdmissionAdmitted
	CallDynamicIndexAdmissionRejected
	CallDynamicIndexAdmissionUnknown
)

// CallDynamicIndexFact records a normal-return dynamic-index fact for a
// placeholder-rooted table path.
type CallDynamicIndexFact struct {
	Table       pathdom.Path
	Site        string
	KeyPresence presence.Value
	KeyValue    product.Value
	Value       product.Value
	Admission   CallDynamicIndexAdmission
}

// CallBranchProofKind classifies a normal-return branch proof.
type CallBranchProofKind uint8

const (
	CallBranchProofPathPresence CallBranchProofKind = iota + 1
	CallBranchProofPathEqual
	CallBranchProofPathNotEqual
)

// CallBranchProof records a must branch proof over placeholder paths.
type CallBranchProof struct {
	Kind     CallBranchProofKind
	Path     pathdom.Path
	Presence presence.Value
	Other    pathdom.Path
}

// CallChannelSelectFactKind classifies a normal-return channel-select fact.
type CallChannelSelectFactKind uint8

const (
	CallChannelSelectFactSelect CallChannelSelectFactKind = iota + 1
	CallChannelSelectFactReceive
	CallChannelSelectFactCase
)

// CallChannelSelectFact records a must channel-select fact over optional
// placeholder paths.
type CallChannelSelectFact struct {
	Select string
	Kind   CallChannelSelectFactKind
	Result pathdom.Path
	Case   pathdom.Path
	Index  int
}

// CallEffectDeltaKind classifies a normal-return effect delta.
type CallEffectDeltaKind uint8

const (
	CallEffectDeltaMutation CallEffectDeltaKind = iota + 1
	CallEffectDeltaEscape
	CallEffectDeltaCall
)

// CallEffectDeltaChange summarizes whether an effect changed its target.
type CallEffectDeltaChange uint8

const (
	CallEffectDeltaChangeBottom CallEffectDeltaChange = iota
	CallEffectDeltaChangeNone
	CallEffectDeltaChangeChanged
	CallEffectDeltaChangeUnknown
)

// CallEffectDelta records a normal-return effect delta for a placeholder path.
type CallEffectDelta struct {
	Target pathdom.Path
	Site   string
	Kind   CallEffectDeltaKind
	Before product.Value
	After  product.Value
	Change CallEffectDeltaChange
}
