package geometry

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// RegionMapper is the mount-authenticated seam between a neutral mount
// formula and the engine's sole support representation.  The mapper is bound
// to one exact mounted runtime at construction; Geometry never accepts a
// caller-supplied manager that could disagree with that runtime.
//
// Mount cannot implement this without depending on engine guard state, so
// the engine owner supplies the conversion callback once at the boundary.
// Returning an unavailable or foreign Mask is a refusal; there is no
// true/empty fallback.
type RegionMapper struct {
	fence   binding.Fence
	manager *guard.Manager
	mapFn   func(witness.Region) (support.Mask, bool)
}

// NewRegionMapper binds one neutral-region conversion to one exact mounted
// runtime and guard universe.  The returned value is immutable; the callback
// is only an adapter for the one neutral-to-engine conversion and cannot
// replace mounted scope authentication.
func NewRegionMapper(mounted witness.Mounted, manager *guard.Manager, mapFn func(witness.Region) (support.Mask, bool)) (RegionMapper, bool) {
	if !mounted.Available() || manager == nil || !manager.Valid(manager.True()) || mapFn == nil {
		return RegionMapper{}, false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() {
		return RegionMapper{}, false
	}
	return RegionMapper{fence: fence, manager: manager, mapFn: mapFn}, true
}

func (mapper RegionMapper) Available() bool {
	return mapper.fence.Available() && mapper.manager != nil && mapper.manager.Valid(mapper.manager.True()) && mapper.mapFn != nil
}

func (mapper RegionMapper) Fence() binding.Fence {
	if !mapper.Available() {
		return binding.Fence{}
	}
	return mapper.fence
}

func (mapper RegionMapper) Manager() *guard.Manager {
	if !mapper.Available() {
		return nil
	}
	return mapper.manager
}

func (mapper RegionMapper) Map(region witness.Region) (support.Mask, bool) {
	if !mapper.Available() || region == nil {
		return support.Mask{}, false
	}
	return mapper.mapFn(region)
}

// Key is a dense physical slot used only inside one exact mounted column or
// arrangement. It is not a logical row identity and must not cross the
// Geometry boundary as one. A single named representation keeps the private
// diagram key generic without making local ordinals part of schema identity.
type Key uint64

func scalarKeyLaw[K scalar.Key]() {}

var _ = scalarKeyLaw[Key]

// Geometry is an immutable solve-local view over one exact mounted runtime.
// It retains no row map or scope map: mounted answers are canonical, and the
// mapper owns support conversion. The manager is the
// exact guard universe captured with the mapper's support representation.
type Geometry struct {
	mounted witness.Mounted
	mapper  RegionMapper
	fence   binding.Fence
}

// New adopts one complete mounted witness and one mapper already bound to
// that exact runtime. A zero mount, unstable/invalid fence, or foreign mapper
// is rejected at construction.
func New(mounted witness.Mounted, mapper RegionMapper) (Geometry, bool) {
	if !mounted.Available() || !mapper.Available() {
		return Geometry{}, false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() || !mapper.Fence().Same(fence) {
		return Geometry{}, false
	}
	return Geometry{mounted: mounted, mapper: mapper, fence: fence}, true
}

// Available reports whether the geometry retains a complete mounted witness,
// mapper, and runtime fence.
func (geometry Geometry) Available() bool {
	return geometry.mounted.Available() && geometry.mapper.Available() && geometry.fence.Available() && geometry.mapper.Fence().Same(geometry.fence) && geometry.mounted.RuntimeFence().Same(geometry.fence)
}

// Fence returns the exact runtime authority captured at construction.
func (geometry Geometry) Fence() binding.Fence {
	if !geometry.Available() {
		return binding.Fence{}
	}
	return geometry.fence
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
}

// Available reports whether both geometry inputs are complete.
func (coordinate Coordinate) Available() bool {
	return coordinate.logical.Available() && coordinate.mask.Valid()
}

// Logical returns the stable, namespaced row identity.
func (coordinate Coordinate) Logical() LogicalKey { return coordinate.logical }

// Dense returns the solve-local physical slot. It must not be persisted or
// used as a cross-relation identity.
func (coordinate Coordinate) Dense() Key { return coordinate.dense }

// Mask returns the exact support partition for the authenticated scope.
func (coordinate Coordinate) Mask() support.Mask { return coordinate.mask }

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

// Mask resolves one authenticated ScopeToken to the engine's sole support
// representation. The mounted witness returns the formula authenticated by
// the exact token; the mapper must convert that formula without introducing a
// second mask language.
func (geometry Geometry) Mask(scope binding.ScopeToken) (support.Mask, bool) {
	if !geometry.Available() || !scope.ValidFor(geometry.fence) {
		return support.Mask{}, false
	}
	region, ok := geometry.mounted.RegionForToken(scope)
	if !ok || region == nil {
		return support.Mask{}, false
	}
	mask, ok := geometry.mapper.Map(region)
	if !ok || !mask.Valid() || mask.Manager() != geometry.mapper.Manager() {
		return support.Mask{}, false
	}
	return mask, true
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
	mask, maskOK := geometry.Mask(cell.Scope())
	if !maskOK {
		return Coordinate{}, false
	}
	return Coordinate{logical: logical, dense: Key(index), mask: mask}, true
}
