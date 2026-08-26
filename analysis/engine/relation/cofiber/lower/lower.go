package lower

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

// Lowering is one physical universe and the declared extent of each neutral
// atom within it.
//
// The extents are owner knowledge. This value composes them; it never decides
// what an atom covers, so two mounts of one declaration agree exactly when
// their owners declared the same extents.
type Lowering struct {
	manager *guard.Manager
	atoms   map[identity.ContentID]support.Mask
}

// New adopts the declared extent of every neutral atom in one physical
// universe. Each extent must be a valid mask of that same manager: a foreign
// or unusable extent refuses the whole lowering rather than leaving one atom
// silently unconstrained.
func New(manager *guard.Manager, atoms map[identity.ContentID]support.Mask) (Lowering, bool) {
	if manager == nil {
		return Lowering{}, false
	}
	declared := make(map[identity.ContentID]support.Mask, len(atoms))
	for id, mask := range atoms {
		if !id.Available() || !mask.Valid() || mask.Manager() != manager {
			return Lowering{}, false
		}
		declared[id] = mask
	}
	return Lowering{manager: manager, atoms: declared}, true
}

// Available reports whether this lowering holds a physical universe.
func (lowering Lowering) Available() bool { return lowering.manager != nil }

// Manager returns the physical universe every mask this lowering produces
// belongs to. It is the manager a cofiber authority must be built with.
func (lowering Lowering) Manager() *guard.Manager {
	if !lowering.Available() {
		return nil
	}
	return lowering.manager
}

// Translate evaluates one sealed region into the mask it denotes. It is the
// callback form cofiber.New accepts.
//
// Terminals answer with the unconstrained and empty masks. Every other region
// is folded from its own canonical rows, so a conjunction is answered by
// evaluation rather than by having been enumerated in advance. A region over an
// atom this lowering was not given refuses: an undeclared extent is never
// approximated by a declared one.
func (lowering Lowering) Translate(value region.Region) (support.Mask, bool) {
	if !lowering.Available() || !value.Available() {
		return support.Mask{}, false
	}
	work := support.New(lowering.manager)
	if work == nil {
		return support.Mask{}, false
	}
	mask, ok := lowering.fold(work, value)
	if !ok || !work.Seal() {
		work.Discard()
		return support.Mask{}, false
	}
	return mask, true
}

// fold evaluates the diagram in its own canonical postorder. Every row's low
// and high references name a terminal or an earlier row, so one left-to-right
// pass resolves each row exactly once and the last row is the root.
//
// A candidate mask becomes readable only once the work seals its pages, so the
// fold checks construction and leaves validity to the single seal in Translate.
func (lowering Lowering) fold(work *support.Work, value region.Region) (support.Mask, bool) {
	if value.IsFalse() {
		return work.False(), true
	}
	if value.IsTrue() {
		return work.True(), true
	}
	rows := value.Nodes()
	if len(rows) == 0 {
		return support.Mask{}, false
	}
	resolved := make([]support.Mask, len(rows))
	for index, row := range rows {
		extent, declared := lowering.atoms[row.Atom.ID()]
		if !row.Atom.Available() || !declared {
			return support.Mask{}, false
		}
		low, lowOK := reference(work, resolved[:index], row.Low)
		high, highOK := reference(work, resolved[:index], row.High)
		if !lowOK || !highOK {
			return support.Mask{}, false
		}
		mask, ok := shannon(work, extent, low, high)
		if !ok {
			return support.Mask{}, false
		}
		resolved[index] = mask
	}
	return resolved[len(resolved)-1], true
}

// shannon composes one decision row as (extent and high) or (not extent and
// low). Expanding on the atom's declared extent rather than on a physical
// variable is what lets an atom stand for any region of the universe.
func shannon(work *support.Work, extent, low, high support.Mask) (support.Mask, bool) {
	positive, ok := work.And(extent, high)
	if !ok {
		return support.Mask{}, false
	}
	complement, ok := work.Not(extent)
	if !ok {
		return support.Mask{}, false
	}
	negative, ok := work.And(complement, low)
	if !ok {
		return support.Mask{}, false
	}
	return work.Or(positive, negative)
}

// reference resolves one transport edge against the terminals and the rows
// already folded. A forward or out-of-range edge is refused rather than read
// as an unresolved zero mask.
func reference(work *support.Work, earlier []support.Mask, value uint32) (support.Mask, bool) {
	switch value {
	case 0:
		return work.False(), true
	case 1:
		return work.True(), true
	}
	index := int(value - 2)
	if index < 0 || index >= len(earlier) {
		return support.Mask{}, false
	}
	return earlier[index], true
}
