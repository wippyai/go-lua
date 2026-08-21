package calltarget

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

// View is the read capability for the call-target plane in one authenticated
// cold Program publication. It retains no copied rows or inverse indexes.
type View struct {
	state programstate.State
}

// NewView binds call-target readers to one authenticated sealed Program state.
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

// Count returns the sealed call-target plane width.
func (view View) Count() (int, bool) {
	if !view.Available() {
		return 0, false
	}
	frozen := view.state.Frozen()
	return Family().Count(&frozen, view.state.CatalogID())
}

// At returns one target by its canonical emitted ordinal.
func (view View) At(index int) (Target, bool) {
	if !view.Available() {
		return Target{}, false
	}
	frozen := view.state.Frozen()
	return Family().At(&frozen, view.state.CatalogID(), index)
}

// ForBody resolves the unique closure target entering body. The sealed plane
// remains the sole authority; the view retains no inverse directory.
func (view View) ForBody(body identity.ContentID) (Target, bool) {
	return view.findUnique(body, func(candidate Target) identity.ContentID {
		return candidate.BodyID()
	})
}

// ForAllocation resolves the unique callable target issued by allocation.
// The sealed plane remains the sole authority; no inverse directory is kept.
func (view View) ForAllocation(allocation identity.ContentID) (Target, bool) {
	return view.findUnique(allocation, func(candidate Target) identity.ContentID {
		return candidate.AllocationID()
	})
}

func (view View) findUnique(id identity.ContentID, key func(Target) identity.ContentID) (Target, bool) {
	if !view.Available() || !id.Available() || key == nil {
		return Target{}, false
	}
	count, published := view.Count()
	if !published {
		return Target{}, false
	}
	var found Target
	for index := 0; index < count; index++ {
		candidate, held := view.At(index)
		if !held || key(candidate) != id {
			continue
		}
		if found.Available() {
			return Target{}, false
		}
		found = candidate
	}
	return found, found.Available()
}
