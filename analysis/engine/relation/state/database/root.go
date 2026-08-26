package database

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Version is one immutable, complete W2 runtime root.  The vector is in the
// exact mounted arrangement order; positions are indexed by the sealed layout
// digest so lookup never reconstructs a key or re-censuses a plan.  Store and
// index roots are retained as their owner values, never copied into a second
// row representation.
type Version struct {
	state *rootState
}

type rootState struct {
	parent            *rootState
	mounted           witness.Mounted
	mountedDigest     identity.ContentID
	fence             binding.Fence
	store             store.Version
	arrangement       arrangement.Plan
	arrangementDigest identity.ContentID
	layouts           []arrangement.Layout
	// stableColumns is the immutable coordinate inventory projected once
	// from the sealed mounted layouts. Successors share this exact slice; the
	// publication hot path never reopens the arrangement or allocates a map.
	stableColumns         []model.ColumnID
	indexes               []index.Version
	contributionDirectory contribution.Directory
	contributionState     contribution.State
	layoutPositions       map[identity.ContentID]int
	revision              uint64
	sealed                bool
}

// newFromStore builds the complete database from the initial store root issued
// by Bootstrap. Keeping this door private prevents a committed successor from
// being resubmitted as a fresh root. Every layout is
// materialized, including layouts with no rows.  A missing layout, foreign
// source, or failed index is a refusal; no partial root is returned.
func newFromStore(mounted witness.Mounted, source store.Version, view geometry.Geometry, scratch *store.ReadScratch) (Version, bool) {
	if !mounted.Available() || !source.Available() || !view.Available() || scratch == nil || !scratch.Available() {
		return Version{}, false
	}
	fence := mounted.RuntimeFence()
	plan := mounted.Arrangement()
	if !fence.Available() || !plan.Available() || !view.ValidFor(mounted) || !source.Fence().Same(fence) || source.MountedDigest() != mounted.Digest() || source.ArrangementDigest() != plan.Digest() {
		return Version{}, false
	}
	if scratch.Manager() != view.Manager() {
		return Version{}, false
	}
	within, ok := view.Universe()
	if !ok || !within.Valid() || within.Manager() != scratch.Manager() {
		return Version{}, false
	}
	layouts := plan.Layouts()
	if layouts == nil {
		return Version{}, false
	}
	stableColumns, stableColumnsOK := deriveStableColumns(mounted, layouts)
	if !stableColumnsOK {
		return Version{}, false
	}
	indexes := make([]index.Version, len(layouts))
	positions := make(map[identity.ContentID]int, len(layouts))
	for position, layout := range layouts {
		if !layout.Available() || !layout.ValidFor(mounted.Fence()) || layout.Digest() == (identity.ContentID{}) {
			return Version{}, false
		}
		if _, duplicate := positions[layout.Digest()]; duplicate {
			return Version{}, false
		}
		built, builtOK := index.New(mounted, source, layout, within, scratch)
		if !builtOK || !built.Available() || built.Layout().Digest() != layout.Digest() || !built.Fence().Same(fence) || !built.Source().Same(source) {
			return Version{}, false
		}
		positions[layout.Digest()] = position
		indexes[position] = built
	}
	contributionDirectory, directoryOK := contribution.NewDirectory(fence)
	contributionState, stateOK := contribution.New(fence)
	if !directoryOK || !stateOK {
		return Version{}, false
	}
	value := sealVersion(Version{state: &rootState{
		mounted: mounted, mountedDigest: mounted.Digest(), fence: fence,
		store: source, arrangement: plan, arrangementDigest: plan.Digest(),
		layouts: append([]arrangement.Layout(nil), layouts...), stableColumns: stableColumns, indexes: indexes,
		contributionDirectory: contributionDirectory,
		contributionState:     contributionState,
		layoutPositions:       positions, revision: 1,
	}})
	if !value.Available() {
		return Version{}, false
	}
	return value, true
}

// Available authenticates complete mounted, store, and layout/index roots.
func (version Version) Available() bool {
	if version.state != nil && version.state.sealed {
		return true
	}
	return version.valid()
}

func (version Version) valid() bool {
	if version.state == nil || !version.state.mounted.Available() || !version.state.store.Available() || !version.state.arrangement.Available() || !version.state.fence.Available() || !version.state.mountedDigest.Available() || !version.state.arrangementDigest.Available() || version.state.mountedDigest != version.state.mounted.Digest() || version.state.fence != version.state.mounted.RuntimeFence() || version.state.store.MountedDigest() != version.state.mountedDigest || version.state.store.ArrangementDigest() != version.state.arrangementDigest || version.state.arrangementDigest != version.state.arrangement.Digest() || version.state.layouts == nil || version.state.stableColumns == nil || !validStableColumns(version.state.stableColumns) || version.state.indexes == nil || len(version.state.layouts) != len(version.state.indexes) || len(version.state.layoutPositions) != len(version.state.layouts) || version.state.revision == 0 {
		return false
	}
	if !version.state.contributionDirectory.Available() || !version.state.contributionState.Available() || !version.state.contributionDirectory.Fence().Same(version.state.fence) || !version.state.contributionState.Fence().Same(version.state.fence) {
		return false
	}
	for position, layout := range version.state.layouts {
		if !layout.Available() || !layout.ValidFor(version.state.mounted.Fence()) {
			return false
		}
		mapped, ok := version.state.layoutPositions[layout.Digest()]
		if !ok || mapped != position {
			return false
		}
		owned := version.state.indexes[position]
		if !owned.Available() || !owned.Layout().Equal(layout) || !owned.Fence().Same(version.state.fence) || !owned.Source().Same(version.state.store) {
			return false
		}
	}
	return true
}

// sealVersion performs the complete mounted/index-root validation once. A
// successful aggregate is immutable, so hot readers use only this proof bit.
func sealVersion(version Version) Version {
	if version.state == nil || !version.valid() {
		return Version{}
	}
	version.state.sealed = true
	return version
}

// Same reports exact immutable publication-root identity.
func (version Version) Same(other Version) bool {
	return version.Available() && other.Available() && version.state == other.state
}

// SuccessorOf proves direct ancestry, not merely equal revision numbers.
func (version Version) SuccessorOf(base Version) bool {
	storeChanged := !version.state.store.Same(base.state.store)
	directoryChanged := !version.state.contributionDirectory.Same(base.state.contributionDirectory)
	stateChanged := !version.state.contributionState.Same(base.state.contributionState)
	return version.Available() && base.Available() && !version.Same(base) && version.state.parent == base.state && version.state.fence.Same(base.state.fence) && version.state.mountedDigest == base.state.mountedDigest && version.state.arrangementDigest == base.state.arrangementDigest && (storeChanged || directoryChanged || stateChanged) && (!storeChanged || version.state.store.SuccessorOf(base.state.store)) && (!directoryChanged || version.state.contributionDirectory.SuccessorOf(base.state.contributionDirectory)) && (!stateChanged || version.state.contributionState.SuccessorOf(base.state.contributionState))
}

// ContributionDirectory returns the immutable owner of invocation handles.
// It is intentionally narrow: transaction reduction may advance this root,
// but callers cannot replace or publish it independently of Version.
func (version Version) ContributionDirectory() contribution.Directory {
	if !version.Available() {
		return contribution.Directory{}
	}
	return version.state.contributionDirectory
}

// ContributionState returns the immutable producer-contribution root owned
// by this database Version.
func (version Version) ContributionState() contribution.State {
	if !version.Available() {
		return contribution.State{}
	}
	return version.state.contributionState
}

// Mounted returns the exact immutable capability captured by the root.
func (version Version) Mounted() witness.Mounted {
	if !version.Available() {
		return witness.Mounted{}
	}
	return version.state.mounted
}

// Store returns the exact committed semantic column root.
func (version Version) Store() store.Version {
	if !version.Available() {
		return store.Version{}
	}
	return version.state.store
}

// Fence returns the exact mounted runtime fence.
func (version Version) Fence() binding.Fence {
	if !version.Available() {
		return binding.Fence{}
	}
	return version.state.fence
}

// MountedDigest returns the capability identity captured at seal/mount time.
func (version Version) MountedDigest() identity.ContentID {
	if !version.Available() {
		return identity.ContentID{}
	}
	return version.state.mountedDigest
}

// ArrangementDigest returns the exact sealed physical arrangement identity.
func (version Version) ArrangementDigest() identity.ContentID {
	if !version.Available() {
		return identity.ContentID{}
	}
	return version.state.arrangementDigest
}

// Arrangement returns the complete immutable mounted plan.
func (version Version) Arrangement() arrangement.Plan {
	if !version.Available() {
		return arrangement.Plan{}
	}
	return version.state.arrangement
}

// Revision advances once per aggregate commit and is never used as identity.
func (version Version) Revision() uint64 {
	if !version.Available() {
		return 0
	}
	return version.state.revision
}

// Layouts returns the canonical mounted layout vector defensively.
func (version Version) Layouts() []arrangement.Layout {
	if !version.Available() {
		return nil
	}
	return append([]arrangement.Layout(nil), version.state.layouts...)
}

// Indexes returns one immutable index root for every mounted layout, in the
// exact Layouts order.  The roots themselves are immutable owner values.
func (version Version) Indexes() []index.Version {
	if !version.Available() {
		return nil
	}
	return append([]index.Version(nil), version.state.indexes...)
}

// Index resolves a mounted layout by its sealed physical digest.
func (version Version) Index(layout arrangement.Layout) (index.Version, bool) {
	if !version.Available() || !layout.Available() || !layout.ValidFor(version.state.mounted.Fence()) {
		return index.Version{}, false
	}
	position, ok := version.state.layoutPositions[layout.Digest()]
	if !ok || !version.state.layouts[position].Equal(layout) {
		return index.Version{}, false
	}
	owned := version.state.indexes[position]
	return owned, owned.Available()
}

// ColumnIDs exposes the canonical mounted semantic catalogue through the
// aggregate owner without exposing physical construction types.
func (version Version) ColumnIDs() []model.ColumnID {
	if !version.Available() {
		return nil
	}
	return version.state.store.ColumnIDs()
}
