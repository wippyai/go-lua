package geometry

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// RegionMapper is the explicit seam between a neutral mount formula and the
// engine's sole support representation.  Mount cannot implement this without
// depending on engine guard state, so the owner of that state injects the
// conversion. Returning an unavailable or foreign Mask is a refusal; there is
// no true/empty fallback.
type RegionMapper interface {
	Map(witness.Region) (support.Mask, bool)
}

// Key is the one engine-local scalar representation of a mounted logical row
// position. It satisfies scalar.Key while remaining distinct from any mount
// locator: callers receive a state key, never a physical address or uint64
// locator. A single named representation also keeps heterogeneous columns
// from instantiating a generic geometry protocol once per domain.
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
	manager *guard.Manager
}

// New adopts one complete mounted witness and one explicit region mapper.
// The mounted witness's exact runtime fence is captured once. A zero mount,
// unstable/invalid fence, or absent mapper is rejected at construction.
func New(mounted witness.Mounted, mapper RegionMapper, manager *guard.Manager) (Geometry, bool) {
	if !mounted.Available() || mapper == nil {
		return Geometry{}, false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() || manager == nil || !manager.Valid(manager.True()) {
		return Geometry{}, false
	}
	return Geometry{mounted: mounted, mapper: mapper, fence: fence, manager: manager}, true
}

// Available reports whether the geometry retains a complete mounted witness,
// mapper, and runtime fence.
func (geometry Geometry) Available() bool {
	return geometry.mounted.Available() && geometry.mapper != nil && geometry.fence.Available() && geometry.manager != nil && geometry.manager.Valid(geometry.manager.True()) && geometry.mounted.RuntimeFence().Same(geometry.fence)
}

// Fence returns the exact runtime authority captured at construction.
func (geometry Geometry) Fence() binding.Fence {
	if !geometry.Available() {
		return binding.Fence{}
	}
	return geometry.fence
}

// Coordinate is the complete state input for one authenticated cell. Key is
// the dense logical row key issued by the mount; Mask is the full scope
// partition. No terminal or value is selected here because a scope can
// contain multiple terminals.
type Coordinate struct {
	key  Key
	mask support.Mask
}

// Available reports whether both geometry inputs are complete.
func (coordinate Coordinate) Available() bool {
	return coordinate.mask.Valid()
}

// Key returns the scope-independent logical row key.
func (coordinate Coordinate) Key() Key { return coordinate.key }

// Mask returns the exact support partition for the authenticated scope.
func (coordinate Coordinate) Mask() support.Mask { return coordinate.mask }

// Key resolves one authenticated CellToken to its dense scalar row key.
// The denominator reference is reconstructed from authenticated token fields
// only; no digest or physical-coordinate derivation is performed.
func (geometry Geometry) Key(cell binding.CellToken) (Key, bool) {
	var zero Key
	if !geometry.Available() || !cell.ValidFor(geometry.fence) {
		return zero, false
	}
	ref, ok := model.NewDenominatorRef(cell.Relation(), cell.Witness().Key())
	if !ok {
		return zero, false
	}
	index, ok := geometry.mounted.RowIndex(ref, cell.Row())
	if !ok || index < 0 {
		return zero, false
	}
	return Key(index), true
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
	if !ok || !mask.Valid() || mask.Manager() != geometry.manager {
		return support.Mask{}, false
	}
	return mask, true
}

// Resolve returns the row key and full scope partition together. It never
// chooses a terminal cell and therefore remains valid for heterogeneous,
// multi-terminal scoped reads.
func (geometry Geometry) Resolve(cell binding.CellToken) (Coordinate, bool) {
	key, keyOK := geometry.Key(cell)
	if !keyOK {
		return Coordinate{}, false
	}
	mask, maskOK := geometry.Mask(cell.Scope())
	if !maskOK {
		return Coordinate{}, false
	}
	return Coordinate{key: key, mask: mask}, true
}
