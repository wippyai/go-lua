package transformer

import "github.com/wippyai/go-lua/analysis/engine/state"

// coordinateFamilySame compares opaque family descriptors by their sealed
// ProductDomain positions. Formal tuple inventories are domain-owned, so a
// lane/family ordinal pair is the canonical identity used by their sorted
// physical layout.
func coordinateFamilySame(left, right state.CoordinateFamily) bool {
	return left.Lane().Ordinal() == right.Lane().Ordinal() && left.Ordinal() == right.Ordinal()
}
