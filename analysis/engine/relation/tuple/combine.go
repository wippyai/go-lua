package tuple

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Combine is the tuple-side consequence of one successful relational join.
// It derives the exact cofiber conjunction and provenance join through the
// sealed Geometry/Mounted authorities. A caller therefore cannot attach a wider
// same-fence scope or unrelated lineage to otherwise authentic cells.
func Combine(mounted witness.Mounted, view geometry.Geometry, left, right Tuple) (Tuple, bool) {
	if !left.ValidFor(mounted) || !right.ValidFor(mounted) || !view.ValidFor(mounted) {
		return Tuple{}, false
	}
	scope, ok := view.Conjoin(left.scope, right.scope)
	if !ok || !scope.ValidFor(mounted.RuntimeFence()) {
		return Tuple{}, false
	}
	lineageAuthority, ok := mounted.Lineage()
	if !ok || lineageAuthority == nil {
		return Tuple{}, false
	}
	lineage, ok := lineageAuthority.Join(left.lineage, right.lineage)
	if !ok {
		return Tuple{}, false
	}
	sources := make([]model.RowID, 0, left.SourceLen()+right.SourceLen())
	sources = append(sources, left.sources...)
	sources = append(sources, right.sources...)
	cells := make([]Cell, 0, left.Len()+right.Len())
	cells = append(cells, left.cells...)
	for _, cell := range right.cells {
		cell.source += uint32(left.SourceLen())
		cells = append(cells, cell)
	}
	return newTuple(mounted, scope, lineage, sources, cells)
}

// Append extends one owner-preserving frame with a redeemed right row.  It is
// the narrow tuple primitive for operators such as Expand whose contract
// says that the left (candidate) scope and lineage remain authoritative.
// The right scope must cover the left scope; otherwise retaining the left
// scope would claim a value outside the right row's authenticated extent.
// Unlike Combine, this operation deliberately does not conjoin lineage or
// scope and never invents a normalized intersection token.
func Append(mounted witness.Mounted, view geometry.Geometry, left, right Tuple) (Tuple, bool) {
	if !left.ValidFor(mounted) || !right.ValidFor(mounted) || !view.ValidFor(mounted) {
		return Tuple{}, false
	}
	if !view.Entails(left.scope, right.scope) {
		return Tuple{}, false
	}
	sources := make([]model.RowID, 0, left.SourceLen()+right.SourceLen())
	sources = append(sources, left.sources...)
	sources = append(sources, right.sources...)
	cells := make([]Cell, 0, left.Len()+right.Len())
	cells = append(cells, left.cells...)
	for _, cell := range right.cells {
		cell.source += uint32(left.SourceLen())
		cells = append(cells, cell)
	}
	return newTuple(mounted, left.scope, left.lineage, sources, cells)
}
