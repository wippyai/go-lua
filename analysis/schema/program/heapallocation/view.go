package heapallocation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

// View is the read capability for the allocation and field planes in one
// authenticated cold Program publication. It retains no copied rows or
// secondary indexes.
type View struct {
	state programstate.State
}

// NewView binds allocation readers to one authenticated sealed Program state.
func NewView(state programstate.State) (View, bool) {
	if !state.Available() {
		return View{}, false
	}
	return View{state: state}, true
}

// Available reports whether the view carries authenticated sealed state.
func (view View) Available() bool {
	return view.state.Available()
}

// AllocationCount returns the sealed allocation-plane width.
func (view View) AllocationCount() (int, bool) {
	if !view.Available() {
		return 0, false
	}
	frozen := view.state.Frozen()
	return AllocationFamily().Count(&frozen, view.state.CatalogID())
}

// AllocationAt returns one allocation by its canonical emitted ordinal.
func (view View) AllocationAt(index int) (Allocation, bool) {
	if !view.Available() {
		return Allocation{}, false
	}
	frozen := view.state.Frozen()
	return AllocationFamily().At(&frozen, view.state.CatalogID(), index)
}

// AllocationForID resolves the unique allocation carrying id. The sealed
// plane remains the sole authority; the view retains no inverse directory.
func (view View) AllocationForID(id identity.ContentID) (Allocation, bool) {
	if !view.Available() || !id.Available() {
		return Allocation{}, false
	}
	count, published := view.AllocationCount()
	if !published {
		return Allocation{}, false
	}
	var found Allocation
	for index := 0; index < count; index++ {
		candidate, held := view.AllocationAt(index)
		if !held || candidate.ID() != id {
			continue
		}
		if found.Available() {
			return Allocation{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

// FieldCount returns the sealed field-plane width.
func (view View) FieldCount() (int, bool) {
	if !view.Available() {
		return 0, false
	}
	frozen := view.state.Frozen()
	return FieldFamily().Count(&frozen, view.state.CatalogID())
}

// FieldAt returns one field by its canonical emitted ordinal.
func (view View) FieldAt(index int) (Field, bool) {
	if !view.Available() {
		return Field{}, false
	}
	frozen := view.state.Frozen()
	return FieldFamily().At(&frozen, view.state.CatalogID(), index)
}
