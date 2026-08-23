package seal

import "github.com/wippyai/go-lua/analysis/schema"

// Surface contributes one declaration phase to a sealed schema. Seal receives
// the immutable view of its own entries and a resolver fenced to earlier
// phases.
type Surface interface {
	Kind() schema.SurfaceKind
	Entries() []schema.Entry
	Seal(view View, sealed Sealed) schema.SealFailure
}

// View is the immutable, indexed declaration set for one surface.
type View struct {
	kind    schema.SurfaceKind
	entries []schema.Entry
	index   map[schema.EntryID]int
}

func (view View) Kind() schema.SurfaceKind {
	return view.kind
}

func (view View) Available() bool {
	return view.kind.Available()
}

func (view View) Count() int {
	return len(view.entries)
}

func (view View) At(position int) (schema.Entry, bool) {
	if position < 0 || position >= len(view.entries) {
		return nil, false
	}
	return view.entries[position], true
}

func (view View) Entries() []schema.Entry {
	if view.entries == nil {
		return nil
	}
	return append([]schema.Entry(nil), view.entries...)
}

func (view View) ByID(id schema.EntryID) (schema.Entry, bool) {
	if view.index == nil {
		return nil, false
	}
	position, ok := view.index[id]
	if !ok || position < 0 || position >= len(view.entries) {
		return nil, false
	}
	return view.entries[position], true
}

// Ordinal resolves one entry identity to its dense position in this view.
// The index is the sealed view's sole identity-to-position authority; unlike
// a caller-maintained lookup, it cannot drift from the immutable entry order.
func (view View) Ordinal(id schema.EntryID) (uint32, bool) {
	if view.index == nil {
		return 0, false
	}
	position, ok := view.index[id]
	if !ok || position < 0 || uint64(position) > uint64(^uint32(0)) {
		return 0, false
	}
	if position >= len(view.entries) {
		return 0, false
	}
	return uint32(position), true
}
