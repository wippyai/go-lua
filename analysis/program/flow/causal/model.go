// Package causal owns Flow's final causal-successor authority.
//
// The seal publishes two typed planes. Edge is the local execution/control
// plane and CallBoundary is the explicit dynamic-call cut.  A Call never
// originates an Edge. Continuation consumers use Successors().At to walk the
// exact union of those two planes; no third graph is retained.
package causal

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/recurrence"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Edge is one immutable local causal transfer. From and To are existing
// canonical Terms. Decision is zero for an unguarded transfer and is an
// existing Select, Branch, or Loop Term otherwise. Mu is an existing Label or
// Loop recurrence head; an empty reset interval is still a real Mu edge. A
// self-edge is valid only when Mu is nonzero, preserving its recurrence
// witness even when that witness has an empty reset interval.
type Edge struct {
	From     keyspace.Term
	To       keyspace.Term
	Decision keyspace.Term
	Truth    bool
	Mu       keyspace.Term
}

// CallBoundary is the typed dynamic-call plane. Normal is the exact normal
// resume term when present. Other is populated only for the Select-left
// normal split and is that Select's alternate operand; it is never a generic
// parallel arm. TailReturn is the terminal Return Outcome for a tail call; a
// tail call has no Normal arm. Throw, Yield, and Cancel are the exact
// owning-Body outcomes and are present for every live Call.
type CallBoundary struct {
	Call       keyspace.Term
	Normal     keyspace.Term
	Other      keyspace.Term
	TailReturn keyspace.Term
	Throw      keyspace.Term
	Yield      keyspace.Term
	Cancel     keyspace.Term
	mode       boundaryMode
}

// boundaryMode is seal-private normal-disposition evidence. The published
// value remains a closed sum: one direct normal, a Select-left split, or a
// terminal tail disposition. It is not a generic arm/operation registry.
type boundaryMode uint8

const (
	boundaryDirect boundaryMode = iota + 1
	boundarySelectAnd
	boundarySelectOr
	boundaryTail
)

// BoundaryArmKind is the closed set of CallBoundary successor arms. Resume,
// SelectTrue, and SelectFalse are the three possible normal dispositions;
// Tail is terminal but remains visible to the combined traversal so a caller
// can account for the complete sealed boundary sum.
type BoundaryArmKind uint8

const (
	// BoundaryLocal identifies a local Edge route in the combined successor
	// plane.  The remaining values identify the closed CallBoundary arms.
	// Keeping local in this same vocabulary means callers need only the
	// semantic arm kind; physical plane identity stays private.
	BoundaryLocal BoundaryArmKind = iota + 1
	BoundaryResume
	BoundarySelectTrue
	BoundarySelectFalse
	BoundaryTail
	BoundaryThrow
	BoundaryYield
	BoundaryCancel
)

// isKnownArm is the single closed-domain fence for the successor arm sum.
// Zero and every value after BoundaryCancel are malformed; callers must not
// infer plane identity from an out-of-domain integer.
func isKnownArm(arm BoundaryArmKind) bool {
	return arm >= BoundaryLocal && arm <= BoundaryCancel
}

func isLocalArm(arm BoundaryArmKind) bool { return arm == BoundaryLocal }

func isBoundaryArm(arm BoundaryArmKind) bool {
	return arm >= BoundaryResume && arm <= BoundaryCancel
}

// Successor identifies a transition in the exact local-plus-boundary union.
// Arm is the closed semantic route kind. Physical row references remain
// seal-private capabilities and never become part of this value's public
// identity.
type Successor struct {
	From      keyspace.Term
	To        keyspace.Term
	Decision  keyspace.Term
	Truth     bool
	Mu        keyspace.Term
	Arm       BoundaryArmKind
	component keyspace.Term
	ref       successorRef
	refValid  bool

	// routeDigest is the stable semantic route identity.  The physical row
	// ref remains seal-private and is intentionally absent from this value.
	routeDigest identity.ContentID
	route       RouteIdentity
	result      *Result
	// edgeIndex is a private reset-query capability used by continuation. It
	// never crosses the public flow package as an index or identity.
	edgeIndex      uint32
	edgeIndexValid bool
}

func isOutcome(t keyspace.Term) bool {
	return keyspace.TermFamily(t) == keyspace.FamilyOutcome && keyspace.TermOrdinal(t) != 0
}

func isDecision(t keyspace.Term) bool {
	switch keyspace.TermFamily(t) {
	case keyspace.FamilySelect, keyspace.FamilyBranch, keyspace.FamilyLoop:
		return keyspace.TermOrdinal(t) != 0
	default:
		return false
	}
}

type edgeRow struct {
	Edge
	// component is the recurrence-issued canonical cyclic-component head for
	// this final route, or zero for acyclic/cross-component routes. It is not
	// derived from Mu: ordinary intra-SCC routes legitimately have Mu zero.
	component keyspace.Term
	// resetStart/resetPast are ranks in the owning Mu head's immutable stream.
	// interval is meaningful whenever Mu is nonzero, including start == past.
	resetStart uint32
	resetPast  uint32
	// resetDigest/resetCount are the canonical semantic reset relation. The
	// range remains the sole storage for the relation; these scalars are only
	// the sealed route-key witness needed for O(1) route identity queries.
	resetDigest identity.ContentID
	resetCount  uint32
}

type boundaryRow struct {
	CallBoundary
	// components are row-owned membership scalars for the closed boundary arm
	// sum. No successor index duplicates or reconstructs this projection.
	components [BoundaryCancel + 1]keyspace.Term
	proofs     [BoundaryCancel + 1]boundaryRecurrenceProof
	// refs are sealed O(1) arm rows populated from the canonical union
	// index. They point back to this row; no successor scan is needed to
	// reissue an arm or its route identity.
	refs [BoundaryCancel + 1]successorRef
}

// boundaryRecurrenceProof is the optional exact-Arc Mu/reset witness for one
// existing CallBoundary arm. The row remains the sole owner: it stores no
// copied reset set, only a range into Result.reset's canonical stream.
type boundaryRecurrenceProof struct {
	mu          keyspace.Term
	resetStart  uint32
	resetPast   uint32
	resetDigest identity.ContentID
	resetCount  uint32
}

type successorRef struct {
	index uint32
	// local is true for an Edge row and false for a CallBoundary arm.
	local bool
	arm   BoundaryArmKind
	// routeDigest is the canonical semantic identity of this route. It is an
	// index payload, not a second route row or a public physical identity.
	routeDigest       identity.ContentID
	semanticPath      identity.ContentID
	semanticResetPath identity.ContentID
	resetMembers      *[]identity.ContentID
	semanticMuPath    identity.ContentID
	guardContext      identity.ContentID
	// guardDecisionPath is the exact semanticpath-issued identity of the
	// guarded decision. It is copied while the private structural path lease is
	// live so RouteGuardProof never reopens that released authority.
	guardDecisionPath identity.ContentID
	// fromPoint/toPoint are the exact SourceControl VertexCatalog path refs
	// for this final route.  They are copied while the catalog lease is live;
	// no later Site/term reconstruction may substitute for them.
	fromPoint identity.ContentID
	toPoint   identity.ContentID
	// planOrdinal is seal-local exact Plan provenance. It is copied from the
	// emitters' ordinal rows into the combined ref only long enough to bind
	// the recurrence hierarchy to final routes, then cleared before Result is
	// published.
	planOrdinal    uint32
	planOrdinalSet bool
	// routeIndexOrdinal is the exact slot stamped when the sorted route
	// directory is sealed. Semantic-path publication uses it to update the
	// existing lookup row, and immutable readers retain it as their O(1)
	// membership proof instead of rebuilding an Edge or Boundary by value.
	routeIndexOrdinal uint32
	// wtoRegion is the sole parent-issued local schedule membership for this
	// final route. It is a semantic ID, never a region row/index capability.
	wtoRegion identity.ContentID
	// diagnostic is seal-private route provenance used only if hierarchy row
	// attachment fails. It is cleared before Result publication.
	diagnostic routeDiagnostic
}

type routeLookup struct {
	digest identity.ContentID
	ref    successorRef
}

// Result is the immutable final causal authority. It retains only local Edge
// rows, typed CallBoundary rows, their compact reset pool, and an index whose
// entries point into one of those two planes. Sourcecontrol and recurrence
// results are never retained.
// edgeStore owns the local typed plane and its projections. Keeping these
// arrays behind a child store makes it impossible for Result to become a
// second, wide authority assembled from unrelated relations.
type edgeStore struct {
	// Body/activation arrays are query projections over rows, never another
	// causal row authority.
	rows              []edgeRow
	bodyRanges        []range32
	bodyIndexes       []uint32
	activationRanges  []range32
	activationIndexes []uint32
	activationRoots   []bool
}

type boundaryStore struct {
	// callSlots is the typed Call lookup projection over rows.
	rows      []boundaryRow
	callSlots []uint32
}

type resetStore struct {
	streams      []keyspace.Term
	headRanges   [keyspace.FamilyCount][]range32
	decisionHead [keyspace.FamilyCount][]keyspace.Term
	decisionRank [keyspace.FamilyCount][]uint32
}

type successorIndex struct {
	// familyCounts remains the sealed source denominator fence. ranges is
	// allocated only for families with at least one outgoing route; each
	// active plane is dense by that family's ordinal.
	familyCounts [keyspace.FamilyCount]uint32
	planes       [keyspace.FamilyCount]successorPlane
	refs         []successorRef
	// writeCommitRefs is a dense owner projection from authored Write ordinal
	// to the existing local Successor ref emitted by the assignment commit
	// chain. It is not a second relation or edge plane.
	writeCommitRefs []successorRef
}

// pendingWTORoute is seal-local provenance for one existing final successor.
// It is aligned to successorIndex.refs only until Flow exchanges private graph
// coordinates for semantic WTO memberships; it is never a published route
// relation or a retained graph projection.
type pendingWTORoute struct {
	from     uint32
	to       uint32
	fromPath identity.ContentID
	toPath   identity.ContentID
}

type boundRouteRow struct {
	fromPath   identity.ContentID
	toPath     identity.ContentID
	diagnostic routeDiagnostic
}

type successorPlane struct {
	denominator uint32
	ranges      []range32
}

// Result is the immutable final causal authority. It retains exactly two
// typed planes, one combined index over their union, and narrow owner-keyed
// projections over existing successor refs. Sourcecontrol and recurrence
// inputs are never retained.
type Result struct {
	edges      edgeStore
	boundaries boundaryStore
	reset      resetStore
	index      successorIndex
	// sites is a derived sorted lookup over the existing sourcecontrol fence,
	// successor authority, and sealed body-terminal Outcome coordinates. It is
	// not a third edge plane.
	sites siteStore
	// routeIndex is a sorted inverse over existing successor refs. It retains
	// no endpoint/CFG copy; only the stable digest and the existing ref are
	// needed for logarithmic semantic resolution.
	routeIndex  []routeLookup
	routesReady bool
	// rowsSealed records that sealRows has proven every retained Edge and
	// CallBoundary row in its final post-route form. The two planes are
	// immutable from that point: later phases stamp only the per-arm
	// successorRef payloads, which carry no structural row obligation. A read
	// after this flag therefore projects a proven row instead of re-deriving
	// its well-formedness per successor reference.
	rowsSealed bool
	// components is the exact ordered recurrence-issued component directory.
	// It retains only heads, never node/arc/SCC topology, and seeds Flow.Local.
	components     []keyspace.Term
	componentIndex map[keyspace.Term]uint32
	// componentPaths are Source/Flow-issued semantic region paths, parallel to
	// the recurrence component directory. Exact owner provenance remains on
	// Result; these paths are the portable component preimage.
	componentPaths []identity.ContentID
	// structuralPaths is the sole semanticpath-owned projection for authored
	structuralPaths *semanticpath.CausalPaths
	local           localStore
	wto             wtoStore
	// pendingWTO is the recurrence-issued, owner-private node bracket stream.
	// It exists only between causal sealing and Flow's semantic-path
	// publication; no public query observes it.
	pendingWTO         recurrence.HierarchyProof
	pendingNodeSites   [][]uint32
	pendingWTORoutes   []pendingWTORoute
	pendingVertexPaths []identity.ContentID
	boundRouteRows     []boundRouteRow
	outcomePhasePaths  map[keyspace.Term]identity.ContentID

	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
}

type range32 struct{ start, end uint32 }

// available is the single publication fence for every typed query. A zero
// value (or partially assembled Result) may still contain plausible arrays
// while it is not an authority; all four identities must be present before
// any retained row or index is observable.
func (r *Result) available() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available()
}
