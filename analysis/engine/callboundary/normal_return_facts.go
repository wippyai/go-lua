// Package callboundary defines concrete payload carriers that cross generic
// call boundaries.
package callboundary

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// NormalReturnFacts is the boundary payload schema for facts that hold on a
// normal return and can cross function boundaries through placeholder paths.
// State-lane behavior stays owned by state, summary, and fact application.
type NormalReturnFacts struct {
	PathRefinements   []PathValueFact
	PathStaticMembers []PathStaticMemberFact
	DynamicIndexFacts []DynamicIndexFact
	BranchProofs      []BranchProof
	ChannelSelects    []ChannelSelectFact
	EffectDeltas      []EffectDelta
	EscapeEvents      []EscapeEventFact
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

// BranchProof records a must branch proof over placeholder paths.
type BranchProof struct {
	Kind     pathevidence.BranchProofKind
	Path     pathdom.Path
	Presence presence.Value
	Other    pathdom.Path
}

// ChannelSelectFact records a must channel-select fact with stable caller-provided IDs.
type ChannelSelectFact struct {
	Select channelselectfact.ID
	Kind   channelselectfact.Kind
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

// EscapeEventKind orders cross-boundary escape/seal strength for placeholder
// paths. Larger values dominate smaller values for the same target scope.
type EscapeEventKind uint8

const (
	EscapeEventNone EscapeEventKind = iota
	EscapeEventBorrow
	EscapeEventRetain
	EscapeEventStore
	EscapeEventSend
	EscapeEventExport
	EscapeEventOpaque
)

// EscapeEventFact records a compressed escape/seal event for a placeholder
// target path. Recursive means the event applies to the entire target subtree.
type EscapeEventFact struct {
	Target    pathdom.Path
	Kind      EscapeEventKind
	Recursive bool
}
