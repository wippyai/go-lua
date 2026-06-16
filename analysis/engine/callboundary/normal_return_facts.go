// Package callboundary defines concrete payload carriers that cross generic
// call boundaries.
package callboundary

import (
	"strings"

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
	PathInvalidations []PathInvalidationFact
	DynamicIndexFacts []DynamicIndexFact
	BranchProofs      []BranchProof
	ChannelSelects    []ChannelSelectFact
	FrozenTables      []FrozenTableFact
	EffectDeltas      []EffectDelta
	EscapeEvents      []EscapeEventFact
	StoreRelations    []StoreRelationFact
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

// PathInvalidationFact records that descendants below a placeholder path were
// invalidated by a normal-returning call.
type PathInvalidationFact struct {
	Path pathdom.Path
}

const pathInvalidationEffectSite = effectdelta.Site("path-descendant-invalidation")

func PathInvalidationEffectSite() effectdelta.Site {
	return pathInvalidationEffectSite
}

func IsPathInvalidationEffectSite(site effectdelta.Site) bool {
	return site == pathInvalidationEffectSite
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
	Select     channelselectfact.ID
	Kind       channelselectfact.Kind
	Result     pathdom.Path
	Case       pathdom.Path
	Index      int
	HasDefault bool
}

// FrozenTableFact records a must frozen-table fact for a placeholder path.
type FrozenTableFact struct {
	Target pathdom.Path
}

const frozenTableEffectSite = effectdelta.Site("frozen-table")

func FrozenTableEffectSite() effectdelta.Site {
	return frozenTableEffectSite
}

func IsFrozenTableEffectSite(site effectdelta.Site) bool {
	return site == frozenTableEffectSite
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

// StoreRelationFact records that Source is stored into Into on a normal return.
// It preserves manifest-level ownership.Store{Param,Into} relation evidence
// while behavior remains carried by EscapeEvents and PathInvalidations.
type StoreRelationFact struct {
	Source pathdom.Path
	Into   pathdom.Path
}

const escapeEventEffectSitePrefix = "escape-event."

// EscapeEventEffectSite is the reserved effect-delta site used while a
// placeholder escape event is materialized inside point state.
func EscapeEventEffectSite(kind EscapeEventKind, recursive bool) effectdelta.Site {
	name := escapeEventKindName(kind)
	if name == "" {
		name = "opaque"
	}
	if recursive {
		name += ".recursive"
	}
	return effectdelta.Site(escapeEventEffectSitePrefix + name)
}

// EscapeEventFromEffectSite recognizes effect-delta sites produced by
// EscapeEventEffectSite.
func EscapeEventFromEffectSite(site effectdelta.Site) (EscapeEventKind, bool, bool) {
	name, ok := strings.CutPrefix(string(site), escapeEventEffectSitePrefix)
	if !ok {
		return EscapeEventNone, false, false
	}
	recursive := false
	if base, ok := strings.CutSuffix(name, ".recursive"); ok {
		name = base
		recursive = true
	}
	kind, ok := escapeEventKindByName(name)
	return kind, recursive, ok
}

func escapeEventKindName(kind EscapeEventKind) string {
	switch kind {
	case EscapeEventBorrow:
		return "borrow"
	case EscapeEventRetain:
		return "retain"
	case EscapeEventStore:
		return "store"
	case EscapeEventSend:
		return "send"
	case EscapeEventExport:
		return "export"
	case EscapeEventOpaque:
		return "opaque"
	default:
		return ""
	}
}

func escapeEventKindByName(name string) (EscapeEventKind, bool) {
	switch name {
	case "borrow":
		return EscapeEventBorrow, true
	case "retain":
		return EscapeEventRetain, true
	case "store":
		return EscapeEventStore, true
	case "send":
		return EscapeEventSend, true
	case "export":
		return EscapeEventExport, true
	case "opaque":
		return EscapeEventOpaque, true
	default:
		return EscapeEventNone, false
	}
}
