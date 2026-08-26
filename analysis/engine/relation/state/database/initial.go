package database

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// Bootstrap seals the one fresh W2 database root for an exact mounted
// runtime.  The mounted catalogue and Geometry are the only inputs: this
// owner constructs every physical column, the complete semantic store, and
// every arrangement index before returning one immutable aggregate.
//
// In particular, a store.Version is intentionally not an input.  Store roots
// are publication successors and therefore cannot be re-submitted as fresh
// database roots by the API shape.
func Bootstrap(mounted witness.Mounted, view geometry.Geometry) (Version, bool) {
	if !mounted.Available() || !view.ValidFor(mounted) {
		return Version{}, false
	}
	manager := view.Manager()
	if manager == nil || !manager.Valid(manager.True()) {
		return Version{}, false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() {
		return Version{}, false
	}

	schemas := mounted.Columns()
	if len(schemas) == 0 || schemas == nil {
		return Version{}, false
	}

	initial := make([]column.Version, len(schemas))
	for position, schema := range schemas {
		owned, ok := column.NewColumn(schema, fence, manager)
		if !ok || owned == nil || !owned.Available() {
			return Version{}, false
		}
		version := owned.Initial()
		if !version.Available() || version.Schema() != schema || !version.Fence().Same(fence) || version.Guards() != manager {
			return Version{}, false
		}
		initial[position] = version
	}

	source, ok := store.NewVersion(mounted, initial)
	if !ok || !source.Available() || !source.Fence().Same(fence) {
		return Version{}, false
	}
	scratch := store.NewReadScratch(manager)
	if scratch == nil || !scratch.Available() {
		return Version{}, false
	}
	return newFromStore(mounted, source, view, scratch)
}
