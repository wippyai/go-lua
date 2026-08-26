package geometry

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Key is a dense physical slot used only inside one exact mounted column or
// arrangement. It is not a logical row identity and must not cross the
// Geometry boundary as one. A single named representation keeps the private
// diagram key generic without making local ordinals part of schema identity.
type Key uint64

func scalarKeyLaw[K scalar.Key]() {}

var _ = scalarKeyLaw[Key]

// Geometry is an immutable solve-local view over one exact mounted runtime.
// It retains no row map and no scope map: Mounted owns scope issuance, while
// the cofiber authority owns the one sealed logical/physical translation and
// normalizes every physical partition before it can cross to logical state.
type Geometry struct {
	mounted witness.Mounted
	scopes  cofiber.Authority
	fence   binding.Fence
}

// New adopts one complete mounted witness and one cofiber authority already
// bound to that exact runtime.  A zero mount, foreign authority, or a raw
// translator callback is rejected here: translation must have been sealed by
// cofiber.New during Bootstrap.
func New(mounted witness.Mounted, scopes cofiber.Authority) (Geometry, bool) {
	if !mounted.Available() || !scopes.ValidFor(mounted) {
		return Geometry{}, false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() || !scopes.Fence().Same(fence) {
		return Geometry{}, false
	}
	return Geometry{mounted: mounted, scopes: scopes, fence: fence}, true
}

// Available reports whether the geometry retains a complete mounted witness,
// cofiber authority, and runtime fence.
func (geometry Geometry) Available() bool {
	return geometry.mounted.Available() && geometry.scopes.Available() && geometry.fence.Available() && geometry.scopes.Fence().Same(geometry.fence) && geometry.mounted.RuntimeFence().Same(geometry.fence)
}

// ValidFor reports whether this geometry redeems the exact mounted artifact
// supplied by a consumer.  RuntimeFence only names the semantic token
// namespace; sibling mounts can deliberately share it while carrying a
// different address book or arrangement.  Geometry therefore compares the
// complete mounted identity and its physical arrangement before a caller
// crosses into state or an operator.
func (geometry Geometry) ValidFor(mounted witness.Mounted) bool {
	if !geometry.Available() || !mounted.Available() || !geometry.mounted.Same(mounted) || !geometry.scopes.ValidFor(mounted) {
		return false
	}
	want := geometry.mounted.Arrangement()
	got := mounted.Arrangement()
	return want.Available() && got.Available() && want.Digest() == got.Digest()
}

// Fence returns the exact runtime authority captured at construction.
func (geometry Geometry) Fence() binding.Fence {
	if !geometry.Available() {
		return binding.Fence{}
	}
	return geometry.fence
}

// Manager returns the exact guard universe owned by this geometry view. It
// is the only way a sibling state owner obtains the support manager needed to
// build a complete immutable aggregate; callers cannot replace the manager.
func (geometry Geometry) Manager() *guard.Manager {
	if !geometry.Available() {
		return nil
	}
	return geometry.scopes.Manager()
}

// Universe returns the immutable unconstrained support region for this exact
// geometry. Aggregate bootstrap uses it to materialize empty arrangement
// roots; it is not a default semantic value and is never used for a missing
// row or a failed scope lookup.
func (geometry Geometry) Universe() (support.Mask, bool) {
	manager := geometry.Manager()
	if manager == nil {
		return support.Mask{}, false
	}
	return support.True(manager)
}

// LogicalKey is the stable logical identity of one mounted row. It carries
// the relation namespace together with the owner-issued RowID, so equal local
// positions from different witnesses or denominators cannot compare equal.
// Denominators prove membership but do not assign row addresses. The
// constructor validates ownership relationships but does not mint a mount
// capability; Geometry.Resolve remains the authenticated issuance door.
type LogicalKey struct {
	relation model.RelationID
	row      model.RowID
}

// NewLogicalKey constructs a namespaced logical row identity. Callers that
// need a mounted proof must use Geometry.Resolve; this value is only the
// stable identity projection after the proof has been checked.
func NewLogicalKey(relation model.RelationID, row model.RowID) (LogicalKey, bool) {
	if !relation.Available() || !row.Available() || row.Relation() != relation {
		return LogicalKey{}, false
	}
	return LogicalKey{relation: relation, row: row}, true
}

func (key LogicalKey) Available() bool {
	return key.relation.Available() && key.row.Available() && key.row.Relation() == key.relation
}

func (key LogicalKey) Relation() model.RelationID { return key.relation }
func (key LogicalKey) Row() model.RowID           { return key.row }

// Coordinate is the complete state input for one authenticated cell. The
// Logical key is the only stable identity. Dense is a private physical slot
// used to address state diagrams, and Mask is the full scope partition. No
// terminal or value is selected here because a scope can contain multiple
// terminals.
type Coordinate struct {
	logical LogicalKey
	dense   Key
	mask    support.Mask
	scope   witness.Scope
}

// Available reports whether both geometry inputs are complete.
func (coordinate Coordinate) Available() bool {
	return coordinate.logical.Available() && coordinate.mask.Valid() && coordinate.scope.Available()
}

// Logical returns the stable, namespaced row identity.
func (coordinate Coordinate) Logical() LogicalKey { return coordinate.logical }

// Dense returns the solve-local physical slot. It must not be persisted or
// used as a cross-relation identity.
func (coordinate Coordinate) Dense() Key { return coordinate.dense }

// Mask returns the exact support partition for the authenticated scope.
func (coordinate Coordinate) Mask() support.Mask { return coordinate.mask }

// Scope returns the canonical runtime Scope for Coordinate.Mask.  It is not
// necessarily the declaration token that originated the cell: Boolean diagram
// partitioning may have produced an intersection, union, or difference whose
// exact scope is owned by the cofiber normalization authority.
func (coordinate Coordinate) Scope() witness.Scope { return coordinate.scope }

// LogicalKey resolves one authenticated CellToken to its stable, namespaced
// logical row identity. The denominator witness proves membership; no digest
// or physical-coordinate derivation is performed.
func (geometry Geometry) LogicalKey(cell binding.CellToken) (LogicalKey, bool) {
	var zero LogicalKey
	if !geometry.Available() || !cell.ValidFor(geometry.fence) {
		return zero, false
	}
	ref, ok := model.NewDenominatorRef(cell.Relation(), cell.Witness().Key())
	if !ok {
		return zero, false
	}
	admitted, ok := geometry.mounted.Denominator(ref)
	if !ok || !admitted.Same(cell.Witness()) || !admitted.Contains(cell.Row()) {
		return zero, false
	}
	if _, ok := geometry.mounted.RowIndex(cell.Relation(), cell.Row()); !ok {
		return zero, false
	}
	return NewLogicalKey(cell.Relation(), cell.Row())
}

// Mask resolves one authenticated ScopeToken through the sealed cofiber
// authority.  Geometry never accepts or retains a raw Region mapper.
func (geometry Geometry) Mask(scope binding.ScopeToken) (support.Mask, bool) {
	if !geometry.Available() || !scope.ValidFor(geometry.fence) {
		return support.Mask{}, false
	}
	logical, logicalOK := geometry.mounted.ScopeForToken(scope)
	if !logicalOK {
		return support.Mask{}, false
	}
	return geometry.scopes.Mask(logical)
}

// Normalize reifies one exact physical support partition as its canonical
// mounted runtime Scope.  It is the only physical-to-logical scope crossing;
// callers never derive a token from a mask identity themselves.
func (geometry Geometry) Normalize(mask support.Mask) (witness.Scope, bool) {
	if !geometry.Available() {
		return witness.Scope{}, false
	}
	return geometry.scopes.Normalize(mask)
}

// Conjoin returns the one normalized runtime Scope for the exact physical
// intersection of two authenticated fibers.  It is intentionally a logical
// result-only surface: operators never receive the intermediate Mask.
func (geometry Geometry) Conjoin(left, right witness.Scope) (witness.Scope, bool) {
	if !geometry.Available() {
		return witness.Scope{}, false
	}
	return geometry.scopes.Conjoin(left, right)
}

// Entails reports exact physical inclusion of two authenticated normalized
// fibers.  Selection uses this instead of the declared-only witness Region
// algebra, which cannot express arbitrary partition differences.
func (geometry Geometry) Entails(premise, conclusion witness.Scope) bool {
	return geometry.Available() && geometry.scopes.Entails(premise, conclusion)
}

// Scope resolves and normalizes an authenticated scope token.  Runtime
// state must use this result rather than carrying an unnormalized declared
// scope beside a diagram partition.
func (geometry Geometry) Scope(token binding.ScopeToken) (witness.Scope, bool) {
	if !geometry.Available() || !token.ValidFor(geometry.fence) {
		return witness.Scope{}, false
	}
	logical, logicalOK := geometry.mounted.ScopeForToken(token)
	if !logicalOK {
		return witness.Scope{}, false
	}
	mask, maskOK := geometry.scopes.Mask(logical)
	if !maskOK {
		return witness.Scope{}, false
	}
	return geometry.scopes.Normalize(mask)
}

// Resolve returns the row key and full scope partition together. It never
// chooses a terminal cell and therefore remains valid for heterogeneous,
// multi-terminal scoped reads.
func (geometry Geometry) Resolve(cell binding.CellToken) (Coordinate, bool) {
	logical, logicalOK := geometry.LogicalKey(cell)
	if !logicalOK {
		return Coordinate{}, false
	}
	index, indexOK := geometry.mounted.RowIndex(logical.Relation(), logical.Row())
	if !indexOK || index < 0 {
		return Coordinate{}, false
	}
	row, rowOK := geometry.mounted.RowAt(logical.Relation(), index)
	if !rowOK || row != logical.Row() {
		return Coordinate{}, false
	}
	scope, scopeOK := geometry.Scope(cell.Scope())
	if !scopeOK {
		return Coordinate{}, false
	}
	mask, maskOK := geometry.scopes.Mask(scope)
	if !maskOK {
		return Coordinate{}, false
	}
	return Coordinate{logical: logical, dense: Key(index), mask: mask, scope: scope}, true
}
