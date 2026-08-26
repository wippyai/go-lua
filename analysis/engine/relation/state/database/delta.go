package database

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Delta is the one database publication difference.  It pairs the exact
// store delta with one index delta for every mounted layout.  An index delta
// may be empty when that layout's root is unchanged, but it is still present
// in the vector so a consumer cannot mistake an uncomputed layout for an
// unrelated one.
type Delta struct {
	base         Version
	next         Version
	source       store.Delta
	indexes      []index.Delta
	contribution contribution.Delta
	storeEmpty   bool
	semantic     bool
	lineage      bool
	sealed       bool
}

// Available authenticates the complete atomic transition.  The store and
// every index must advance from the exact roots retained by Base; no partial
// vector, foreign arrangement, or second candidate can be admitted.
func (delta Delta) Available() bool {
	if delta.sealed {
		return true
	}
	return delta.valid()
}

func (delta Delta) valid() bool {
	if !delta.base.Available() || !delta.next.Available() || !delta.next.SuccessorOf(delta.base) ||
		(!delta.storeEmpty && (!delta.source.Available() || !delta.source.Base().Same(delta.base.Store()) || !delta.source.Next().Same(delta.next.Store()))) ||
		delta.semantic != (len(delta.source.SemanticColumnIDs()) != 0) || delta.lineage != (len(delta.source.LineageColumnIDs()) != 0) {
		return false
	}
	if delta.base.MountedDigest() != delta.next.MountedDigest() || delta.base.ArrangementDigest() != delta.next.ArrangementDigest() || !delta.base.Fence().Same(delta.next.Fence()) {
		return false
	}
	if delta.storeEmpty {
		if !delta.contribution.Available() || !delta.contribution.Base().Same(delta.base.ContributionState()) || !delta.contribution.Next().Same(delta.next.ContributionState()) {
			return false
		}
		// A contribution-only publication advances no store or index root.  It
		// deliberately carries no fabricated index deltas; prove instead that
		// the complete index projection is shared by the two database roots.
		if len(delta.indexes) != 0 || len(delta.base.state.indexes) != len(delta.next.state.indexes) {
			return false
		}
		for position := range delta.base.state.indexes {
			if !delta.next.state.indexes[position].Same(delta.base.state.indexes[position]) {
				return false
			}
		}
	} else if !delta.next.ContributionState().Same(delta.base.ContributionState()) {
		if !delta.contribution.Available() || !delta.contribution.Base().Same(delta.base.ContributionState()) || !delta.contribution.Next().Same(delta.next.ContributionState()) {
			return false
		}
	} else if delta.contribution.Available() {
		return false
	}
	if !delta.storeEmpty && (delta.source.Base().MountedDigest() != delta.base.MountedDigest() || delta.source.Next().MountedDigest() != delta.next.MountedDigest() || delta.source.Base().ArrangementDigest() != delta.base.ArrangementDigest() || delta.source.Next().ArrangementDigest() != delta.next.ArrangementDigest()) {
		return false
	}
	if delta.storeEmpty {
		return true
	}
	if len(delta.indexes) != len(delta.base.state.indexes) {
		return false
	}
	for position, child := range delta.indexes {
		prior := delta.base.state.indexes[position]
		candidate := delta.next.state.indexes[position]
		if !child.Available() || !child.Base().Same(prior) || !child.Next().Same(candidate) || !candidate.SuccessorOf(prior) ||
			!candidate.Layout().Equal(delta.base.state.layouts[position]) || !candidate.Fence().Same(delta.base.state.fence) ||
			!candidate.Source().Same(delta.next.Store()) {
			return false
		}
	}
	return true
}

// AffectedContributionTargets returns exact (output port, row) targets
// touched by the authenticated contribution transition.
func (delta Delta) AffectedContributionTargets() []contribution.Target {
	if !delta.Available() || !delta.contribution.Available() {
		return nil
	}
	return delta.contribution.AffectedTargets()
}

func sealDelta(delta Delta) Delta {
	if delta.valid() {
		delta.sealed = true
	}
	return delta
}

// Base returns the exact aggregate predecessor.
func (delta Delta) Base() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.base
}

// Next returns the exact aggregate successor.
func (delta Delta) Next() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.next
}

// Source returns the immutable semantic store projection.  Arrangement
// changes are exposed separately through Indexes; this method never exposes
// an internal column or row representation.
func (delta Delta) Source() store.Delta {
	if !delta.Available() {
		return store.Delta{}
	}
	return delta.source
}

// Indexes returns the complete layout-aligned vector defensively.
func (delta Delta) Indexes() []index.Delta {
	if !delta.Available() {
		return nil
	}
	return append([]index.Delta(nil), delta.indexes...)
}

// ChangedColumnIDs returns the store's canonical changed-column set.
func (delta Delta) ChangedColumnIDs() []model.ColumnID {
	if !delta.Available() {
		return nil
	}
	return delta.source.ChangedColumnIDs()
}

// SemanticColumnIDs returns the store columns with actual semantic changes.
func (delta Delta) SemanticColumnIDs() []model.ColumnID {
	if !delta.Available() {
		return nil
	}
	return delta.source.SemanticColumnIDs()
}

// LineageColumnIDs returns columns whose proof lineage advanced without a
// semantic value change.
func (delta Delta) LineageColumnIDs() []model.ColumnID {
	if !delta.Available() {
		return nil
	}
	return delta.source.LineageColumnIDs()
}

// Changes returns one canonical semantic+lineage projection for every changed
// column, including lineage-only columns. Extents retain only private
// geometry key/support data; Mounted and Geometry remain the RowID/Scope
// authorities for runtime consumers.
func (delta Delta) Changes() []store.ColumnChange {
	if !delta.Available() {
		return nil
	}
	return delta.source.Changes()
}

// Change resolves one complete semantic+lineage column projection. A missing
// result means the column was unchanged.
func (delta Delta) Change(id model.ColumnID) (store.ColumnChange, bool) {
	if !delta.Available() {
		return store.ColumnChange{}, false
	}
	return delta.source.Change(id)
}

// SemanticChanged and LineageChanged retain the exact source partition as
// named aggregate facts; they are not inferred from index churn.
func (delta Delta) SemanticChanged() bool { return delta.Available() && delta.semantic }

func (delta Delta) LineageChanged() bool { return delta.Available() && delta.lineage }

// MountedDigest exposes the root identity authenticated by this transition.
func (delta Delta) MountedDigest() identity.ContentID {
	if !delta.Available() {
		return identity.ContentID{}
	}
	return delta.next.MountedDigest()
}

// ArrangementDigest exposes the sealed physical arrangement identity.
func (delta Delta) ArrangementDigest() identity.ContentID {
	if !delta.Available() {
		return identity.ContentID{}
	}
	return delta.next.ArrangementDigest()
}
