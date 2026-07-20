// Package callboundary defines concrete payload carriers that cross generic
// call boundaries.
package callboundary

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// NormalReturnFacts is the boundary payload schema for facts that hold on a
// normal return and can cross function boundaries through placeholder paths.
// State-lane behavior stays owned by state, summary, and fact application.
type NormalReturnFacts struct {
	PathRefinements          []PathValueFact
	PersistentPathWrites     []PathValueFact
	PathStaticMembers        []PathStaticMemberFact
	PathStaticMemberDeltas   []PathStaticMemberDeltaFact
	PathInvalidations        []PathInvalidationFact
	DynamicIndexFacts        []DynamicIndexFact
	KeyMemberships           []KeyMembershipFact
	DynamicValueKeys         []DynamicValueKeyMembershipFact
	DynamicAllValues         []DynamicAllValueKeyMembershipFact
	BranchProofs             []BranchProof
	PathPresenceImplications []PathPresenceImplicationFact
	ChannelSelects           []ChannelSelectFact
	FrozenTables             []FrozenTableFact
	EffectDeltas             []EffectDelta
	EscapeEvents             []EscapeEventFact
	StoreRelations           []StoreRelationFact
	LifecycleFacts           []LifecycleFact
	NumFloors                []NumFloorFact
	NumCeils                 []NumCeilFact
	RelConstraints           []RelConstraintFact
}

// Empty reports whether no normal-return fact lane carries evidence.
func (f NormalReturnFacts) Empty() bool {
	for _, lane := range normalReturnFactLanes {
		if lane.Len(f) != 0 {
			return false
		}
	}
	return true
}

// Append returns f with every normal-return fact lane from other appended.
func (f NormalReturnFacts) Append(other NormalReturnFacts) NormalReturnFacts {
	if other.Empty() {
		return f
	}
	if f.Empty() {
		return other
	}
	return appendNonEmptyNormalReturnFacts(f, other)
}

func appendNonEmptyNormalReturnFacts(f, other NormalReturnFacts) NormalReturnFacts {
	for _, lane := range normalReturnFactLanes {
		f = lane.Append(f, other)
	}
	return f
}

func appendNormalReturnSlice[T any](left, right []T) []T {
	if len(right) == 0 {
		return left
	}
	if len(left) == 0 {
		return right
	}
	out := make([]T, len(left), len(left)+len(right))
	copy(out, left)
	return append(out, right...)
}

// PathValueFact records a pointwise placeholder-path value refinement.
type PathValueFact struct {
	Path  pathdom.Path
	Value product.Value
}

// ProjectPathRefinementValue is the single canonical value rule for a
// pointwise path refinement crossing a function boundary. Bottom and Top carry
// no useful refinement and are omitted; every admitted value loses
// solve-local axes through the product boundary projection.
func ProjectPathRefinementValue(reg *axis.Registry, value product.Value) (product.Value, bool) {
	if reg == nil || product.Equal(reg, value, product.Bottom(reg)) || product.Equal(reg, value, product.Top()) {
		return product.Value{}, false
	}
	return product.ProjectBoundary(reg, value), true
}

// PathStaticMemberFact records a must static-member fact for a placeholder path.
type PathStaticMemberFact struct {
	Path  pathdom.Path
	Value product.Value
}

// PathStaticMemberDeltaFact records a structural static member addition to a
// placeholder path. Required deltas hold on every normal return; non-required
// deltas are may-adds and consumers materialize them as optional members.
type PathStaticMemberDeltaFact struct {
	Path     pathdom.Path
	Value    product.Value
	Required bool
}

// PathPresenceImplicationFact records a must implication between two boundary
// paths on normal return: when Trigger satisfies TriggerPresence or TriggerValue,
// Target has TargetPresence or TargetValue. This carries local value/presence
// correlations such as result.status == "error" => result.error present across
// call boundaries, and preserves shaped target values when the callee proved
// them.
type PathPresenceImplicationFact struct {
	Trigger         pathdom.Path
	TriggerPresence presence.Value
	TriggerValue    product.Value
	HasTriggerValue bool
	Target          pathdom.Path
	TargetPresence  presence.Value
	TargetValue     product.Value
	HasTargetValue  bool
}

// PathInvalidationFact records that descendants below a placeholder argument
// path, or an internal concrete captured path, were invalidated by a
// normal-returning call.
type PathInvalidationFact struct {
	Path                      pathdom.Path
	PreserveStructuralWitness bool
	ClearTarget               bool
}

const pathInvalidationEffectSite = effectdelta.Site("path-descendant-invalidation")
const pathStructuralPreservingInvalidationEffectSite = effectdelta.Site("path-descendant-invalidation:preserve-structural")

func PathInvalidationEffectSite() effectdelta.Site {
	return pathInvalidationEffectSite
}

func PathStructuralPreservingInvalidationEffectSite() effectdelta.Site {
	return pathStructuralPreservingInvalidationEffectSite
}

func IsPathInvalidationEffectSite(site effectdelta.Site) bool {
	return site == pathInvalidationEffectSite
}

func IsPathStructuralPreservingInvalidationEffectSite(site effectdelta.Site) bool {
	return site == pathStructuralPreservingInvalidationEffectSite
}

// DynamicIndexFact records a pointwise dynamic index fact for a placeholder table.
type DynamicIndexFact struct {
	Table     pathdom.Path
	Site      dynamicindex.Site
	KeyPath   pathdom.Path
	ValuePath pathdom.Path
	Value     dynamicindex.Fact
}

// KeyMembershipFact records that Key is proven to be a key of Table on normal
// return.
type KeyMembershipFact struct {
	Key   pathdom.Path
	Table pathdom.Path
}

// DynamicValueKeyMembershipFact records that values written into Container at
// Site are keys of Table on normal return.
type DynamicValueKeyMembershipFact struct {
	Container pathdom.Path
	Site      dynamicindex.Site
	Table     pathdom.Path
}

// DynamicAllValueKeyMembershipFact records that every present value reachable
// through Container is a key of Table on normal return.
type DynamicAllValueKeyMembershipFact struct {
	Container pathdom.Path
	Table     pathdom.Path
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
type EscapeEventKind = escapeevent.Kind

const (
	EscapeEventNone   = escapeevent.KindNone
	EscapeEventBorrow = escapeevent.KindBorrow
	EscapeEventRetain = escapeevent.KindRetain
	EscapeEventStore  = escapeevent.KindStore
	EscapeEventSend   = escapeevent.KindSend
	EscapeEventExport = escapeevent.KindExport
	EscapeEventOpaque = escapeevent.KindOpaque
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

// LifecycleKind classifies a typestate update crossing a call boundary.
type LifecycleKind uint8

const (
	LifecycleNone LifecycleKind = iota
	LifecycleAcquire
	LifecycleTransition
	LifecycleEscape
)

// LifecycleFact records a protocol state-machine update for a placeholder
// target path. The resource identity is resolved from canonical caller path
// evidence when the fact is applied.
type LifecycleFact struct {
	Target     pathdom.Path
	Kind       LifecycleKind
	Protocol   typestate.Protocol
	From       typestate.State
	To         typestate.State
	Obligation typestate.Obligation
}

// NumFloorFact records a proven lower bound for a numeric placeholder path on
// normal return: value(Path) >= Floor.
type NumFloorFact struct {
	Path  pathdom.Path
	Floor int64
}

// NumCeilFact records a proven upper bound for a numeric placeholder path on
// normal return: value(Path) <= Ceil.
type NumCeilFact struct {
	Path pathdom.Path
	Ceil int64
}

// RelOperand is a placeholder operand in a normal-return relational constraint.
// Length operands stand for len(Path); value operands stand for value(Path).
type RelOperand struct {
	Path     pathdom.Path
	IsLength bool
}

// RelConstraintFact records CoA*A + CoB*B - C <= K on normal return. B is
// optional when CoB is zero or B.Path is empty.
type RelConstraintFact struct {
	CoA int64
	A   RelOperand
	CoB int64
	B   RelOperand
	C   RelOperand
	K   int64
}
