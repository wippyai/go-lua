package read

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
)

// Reader is the sole state-to-operator seam. It is an opaque value handle,
// not an interface: sibling packages cannot implement a reader that relays
// rows while lying about its layout, root, or scope authority. Bind accepts
// only an immutable, committed aggregate Version and a layout redeemed from
// that Version. The handle owns no store or index construction and has no
// injectable Source compatibility seam.
type Reader struct {
	value *reader
}

// Available reports whether this handle still redeems one complete committed
// reader. The zero value is intentionally unavailable.
func (handle Reader) Available() bool {
	return handle.value != nil && handle.value.available()
}

func (handle Reader) Layout() arrangement.Layout {
	if !handle.Available() {
		return arrangement.Layout{}
	}
	return handle.value.Layout()
}

func (handle Reader) Type(column model.ColumnID) (model.TypeID, bool) {
	if !handle.Available() {
		return model.TypeID{}, false
	}
	return handle.value.Type(column)
}

// Conjoin and Entails redeem the reader's sealed cofiber authority. They keep
// physical masks inside state: operators receive only normalized runtime
// Scope tokens and cannot reopen declared-only Region algebra.
func (handle Reader) Conjoin(left, right witness.Scope) (witness.Scope, bool) {
	if !handle.Available() {
		return witness.Scope{}, false
	}
	return handle.value.Conjoin(left, right)
}

func (handle Reader) Entails(premise, conclusion witness.Scope) bool {
	return handle.Available() && handle.value.Entails(premise, conclusion)
}

// Owns reports whether candidate is an authenticated row borrowed from this
// exact Reader.  It is a capability predicate only: it does not expose the
// private support mask or allow a sibling package to manufacture a row.
// Tuple/evaluator boundaries use it before accepting a Row value supplied by
// another physical operator.
func (handle Reader) Owns(candidate Row) bool {
	return handle.Available() && candidate != nil && candidate.rowFrom(handle.value) && candidate.Available()
}

type reader struct {
	root             database.Version
	index            index.Version
	layout           arrangement.Layout
	view             geometry.Geometry
	scratch          *store.ReadScratch
	mounted          witness.Mounted
	fence            binding.Fence
	manager          *guard.Manager
	lineageAuthority lineage.Authority
	types            []model.TypeID
}

// Bind authenticates one exact committed aggregate, layout, sealed geometry,
// and reusable read scratch. It rejects foreign managers before selecting an
// index, so an irrelevant index cannot hide a mismatched physical universe.
// A Prepared/CandidateIndexes value cannot enter this API: database.Version
// is the only accepted root type and database.Commit is its publication door.
func Bind(root database.Version, layout arrangement.Layout, view geometry.Geometry, scratch *store.ReadScratch) (Reader, bool) {
	if !root.Available() || !layout.Available() || !view.Available() || scratch == nil || !scratch.Available() {
		return Reader{}, false
	}
	mounted := root.Mounted()
	fence := root.Fence()
	manager := view.Manager()
	if !mounted.Available() || !fence.Available() || !view.ValidFor(mounted) || manager == nil || scratch.Manager() != manager {
		return Reader{}, false
	}
	// Authenticate the complete store catalogue first. This is intentionally
	// before root.Index(layout); a foreign manager in an unrelated relation is
	// still a foreign database and must refuse this reader.
	state := root.Store()
	if !state.Available() || !state.Fence().Same(fence) || state.MountedDigest() != root.MountedDigest() || state.ArrangementDigest() != root.ArrangementDigest() {
		return Reader{}, false
	}
	for _, columnID := range state.ColumnIDs() {
		column, ok := state.Column(columnID)
		if !ok || !column.Available() || column.Guards() != manager || !column.Fence().Same(fence) {
			return Reader{}, false
		}
	}
	owned, ok := root.Index(layout)
	if !ok || !owned.Available() || !owned.Layout().Equal(layout) || !owned.Fence().Same(fence) || !owned.Source().Same(state) {
		return Reader{}, false
	}
	lineageAuthority, lineageOK := mounted.Lineage()
	if !lineageOK || lineageAuthority == nil || !lineageAuthority.Fence().Same(fence) {
		return Reader{}, false
	}
	keyColumns := layout.KeyColumns()
	types := make([]model.TypeID, len(keyColumns))
	for position, columnID := range keyColumns {
		column, columnOK := state.Column(columnID)
		if !columnOK || !column.Available() || column.Relation() != layout.Access().Relation() || column.Guards() != manager || !column.Type().Available() {
			return Reader{}, false
		}
		types[position] = column.Type()
	}
	value := &reader{
		root: root, index: owned, layout: layout, view: view, scratch: scratch,
		mounted: mounted, fence: fence, manager: manager,
		lineageAuthority: lineageAuthority, types: types,
	}
	if !value.available() {
		return Reader{}, false
	}
	return Reader{value: value}, true
}

func (value *reader) available() bool {
	return value != nil && value.root.Available() && value.index.Available() && value.layout.Available() && value.view.Available() && value.scratch != nil && value.scratch.Available() && value.mounted.Available() && value.fence.Available() && value.view.ValidFor(value.mounted) && value.scratch.Manager() == value.view.Manager() && value.index.Source().Same(value.root.Store()) && value.index.Layout().Equal(value.layout) && value.index.Fence().Same(value.fence) && value.lineageAuthority != nil && value.lineageAuthority.Fence().Same(value.fence) && len(value.types) == len(value.layout.KeyColumns())
}

func (value *reader) Layout() arrangement.Layout {
	if !value.available() {
		return arrangement.Layout{}
	}
	return value.layout
}

func (value *reader) Type(column model.ColumnID) (model.TypeID, bool) {
	if !value.available() || !column.Available() || column.Relation() != value.layout.Access().Relation() {
		return model.TypeID{}, false
	}
	owned, ok := value.root.Store().Column(column)
	if !ok || !owned.Available() || owned.Guards() != value.manager || !owned.Fence().Same(value.fence) {
		return model.TypeID{}, false
	}
	return owned.Type(), true
}

// Conjoin returns the canonical physical-fiber conjunction of two scopes
// authenticated by this exact committed reader.  A scope from another mount,
// manager, or unadmitted arena entry is refused by Geometry/cofiber.
func (value *reader) Conjoin(left, right witness.Scope) (witness.Scope, bool) {
	if !value.available() {
		return witness.Scope{}, false
	}
	return value.view.Conjoin(left, right)
}

// Entails reports exact physical inclusion for two scopes authenticated by
// this reader's mounted cofiber authority.  It is the scope-as-data selection
// predicate; it does not invoke mounted neutral Region entailment.
func (value *reader) Entails(premise, conclusion witness.Scope) bool {
	return value.available() && value.view.Entails(premise, conclusion)
}
