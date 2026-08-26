package database

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Prepare is the database staging door. The source Prepared is already
// sealed by store.Prepare; this function redeems its private successor only
// to build actual index successors. No root is published here.
func Prepare(base Version, source store.Prepared, scratch *store.ReadScratch, nextDirectory contribution.Directory, nextState contribution.State, contributionDelta *contribution.Delta) (Prepared, bool) {
	if !base.Available() || !source.Available() || !nextDirectory.Available() || !nextState.Available() || !scratch.Available() || !source.Base().Same(base.state.store) || source.MountedDigest() != base.state.mountedDigest || source.ArrangementDigest() != base.state.arrangementDigest || scratch.Manager() == nil || !nextDirectory.Fence().Same(base.Fence()) || !nextState.Fence().Same(base.Fence()) {
		return Prepared{}, false
	}
	directoryChanged := !nextDirectory.Same(base.state.contributionDirectory)
	stateChanged := !nextState.Same(base.state.contributionState)
	if directoryChanged && (!stateChanged || !nextDirectory.SuccessorOf(base.state.contributionDirectory)) || stateChanged && (contributionDelta == nil || !contributionDelta.Available() || !nextState.SuccessorOf(base.state.contributionState) || !contributionDelta.Base().Same(base.state.contributionState) || !contributionDelta.Next().Same(nextState)) || !stateChanged && contributionDelta != nil {
		return Prepared{}, false
	}
	if source.Empty() {
		if !directoryChanged && !stateChanged {
			return newPrepared(base, source, base, append([]index.Version(nil), base.state.indexes...), nil, nil, false, false, true)
		}
		next := sealVersion(Version{state: &rootState{parent: base.state, mounted: base.state.mounted, mountedDigest: base.state.mountedDigest, fence: base.state.fence, store: base.state.store, arrangement: base.state.arrangement, arrangementDigest: base.state.arrangementDigest, layouts: base.state.layouts, stableColumns: base.state.stableColumns, indexes: base.state.indexes, contributionDirectory: nextDirectory, contributionState: nextState, layoutPositions: base.state.layoutPositions, revision: base.state.revision + 1}})
		if !next.Available() || !next.SuccessorOf(base) {
			return Prepared{}, false
		}
		return newPrepared(base, source, next, append([]index.Version(nil), next.state.indexes...), nil, contributionDelta, false, false, false)
	}
	sourceDelta := source.Delta()
	if !sourceDelta.Available() || !sourceDelta.Base().Same(base.state.store) || !sourceDelta.Next().Available() || !sourceDelta.Next().SuccessorOf(base.state.store) {
		return Prepared{}, false
	}
	// Redeem the stable-coordinate law before any arrangement successor is
	// built. A refused coordinate candidate therefore cannot leave a partially materialized
	// index or reach the aggregate publication door.
	if !stableColumnDelta(base.state.mounted, base.state.stableColumns, sourceDelta) {
		return Prepared{}, false
	}
	nextStore := sourceDelta.Next()
	nextIndexes := make([]index.Version, len(base.state.indexes))
	indexDeltas := make([]index.Delta, len(base.state.indexes))
	semantic, lineage := len(sourceDelta.SemanticColumnIDs()) != 0, len(sourceDelta.LineageColumnIDs()) != 0
	for position, prior := range base.state.indexes {
		next, delta, nextOK := prior.Next(sourceDelta, scratch)
		if !nextOK || !next.Available() || !delta.Available() || !next.SuccessorOf(prior) || !next.Source().Same(nextStore) || !next.Layout().Equal(base.state.layouts[position]) {
			return Prepared{}, false
		}
		nextIndexes[position], indexDeltas[position] = next, delta
	}
	next := sealVersion(Version{state: &rootState{
		parent: base.state, mounted: base.state.mounted, mountedDigest: base.state.mountedDigest,
		fence: base.state.fence, store: nextStore, arrangement: base.state.arrangement,
		arrangementDigest: base.state.arrangementDigest, layouts: base.state.layouts, stableColumns: base.state.stableColumns,
		indexes: nextIndexes, contributionDirectory: nextDirectory, contributionState: nextState, layoutPositions: base.state.layoutPositions, revision: base.state.revision + 1,
	}})
	if !next.Available() || !next.SuccessorOf(base) {
		return Prepared{}, false
	}
	return newPrepared(base, source, next, nextIndexes, indexDeltas, contributionDelta, semantic, lineage, false)
}

func newPrepared(base Version, source store.Prepared, next Version, indexes []index.Version, deltas []index.Delta, contributionDelta *contribution.Delta, semantic, lineage, noop bool) (Prepared, bool) {
	value := sealPrepared(Prepared{base: base, source: source, next: next, indexes: indexes, indexDeltas: deltas, contributionDelta: contributionDelta, semantic: semantic, lineage: lineage, noop: noop, mountedDigest: base.MountedDigest(), arrangementDigest: base.ArrangementDigest()})
	if !value.Available() {
		return Prepared{}, false
	}
	return value, true
}

// Prepared is opaque until Commit. It contains the store candidate and the
// actual candidate index roots, not a recipe or callback that recomputes them.
type Prepared struct {
	base              Version
	source            store.Prepared
	next              Version
	indexes           []index.Version
	indexDeltas       []index.Delta
	contributionDelta *contribution.Delta
	mountedDigest     identity.ContentID
	arrangementDigest identity.ContentID
	semantic          bool
	lineage           bool
	noop              bool
	sealed            bool
}

// Available re-authenticates the complete private candidate.
func (prepared Prepared) Available() bool {
	if prepared.sealed {
		return true
	}
	return prepared.valid()
}

func (prepared Prepared) valid() bool {
	if !prepared.base.Available() || !prepared.source.Available() || !prepared.next.Available() || !prepared.mountedDigest.Available() || !prepared.arrangementDigest.Available() || prepared.mountedDigest != prepared.base.MountedDigest() || prepared.arrangementDigest != prepared.base.ArrangementDigest() || !prepared.source.Base().Same(prepared.base.Store()) || prepared.source.MountedDigest() != prepared.mountedDigest || prepared.source.ArrangementDigest() != prepared.arrangementDigest || len(prepared.indexes) != len(prepared.base.state.indexes) {
		return false
	}
	if prepared.noop {
		return prepared.base.Same(prepared.next) && len(prepared.indexDeltas) == 0 && !prepared.semantic && !prepared.lineage && prepared.source.Empty()
	}
	if prepared.source.Empty() {
		return prepared.next.SuccessorOf(prepared.base) && !prepared.semantic && !prepared.lineage && len(prepared.indexDeltas) == 0 && len(prepared.indexes) == len(prepared.base.state.indexes) && prepared.contributionDelta != nil && prepared.contributionDelta.Available() && prepared.contributionDelta.Base().Same(prepared.base.ContributionState()) && prepared.contributionDelta.Next().Same(prepared.next.ContributionState())
	}
	if !prepared.next.SuccessorOf(prepared.base) || len(prepared.indexDeltas) != len(prepared.indexes) || !prepared.semantic && !prepared.lineage {
		return false
	}
	if !prepared.next.ContributionState().Same(prepared.base.ContributionState()) {
		if prepared.contributionDelta == nil || !prepared.contributionDelta.Available() || !prepared.contributionDelta.Base().Same(prepared.base.ContributionState()) || !prepared.contributionDelta.Next().Same(prepared.next.ContributionState()) {
			return false
		}
	} else if prepared.contributionDelta != nil {
		return false
	}
	sourceDelta := prepared.source.Delta()
	if !sourceDelta.Available() || !sourceDelta.Base().Same(prepared.base.Store()) || !sourceDelta.Next().Same(prepared.next.Store()) || prepared.semantic != (len(sourceDelta.SemanticColumnIDs()) != 0) || prepared.lineage != (len(sourceDelta.LineageColumnIDs()) != 0) {
		return false
	}
	for position, candidate := range prepared.indexes {
		prior := prepared.base.state.indexes[position]
		if !candidate.Available() || !candidate.SuccessorOf(prior) || !candidate.Source().Same(prepared.next.Store()) || !candidate.Layout().Equal(prepared.base.state.layouts[position]) || !prepared.indexDeltas[position].Available() || !prepared.indexDeltas[position].Base().Same(prior) || !prepared.indexDeltas[position].Next().Same(candidate) {
			return false
		}
	}
	return true
}

func sealPrepared(prepared Prepared) Prepared {
	if prepared.valid() {
		prepared.sealed = true
	}
	return prepared
}

// Base returns the exact aggregate predecessor.
func (prepared Prepared) Base() Version {
	if !prepared.Available() {
		return Version{}
	}
	return prepared.base
}

// CandidateIndexes returns the actual sealed index successors, defensively.
func (prepared Prepared) CandidateIndexes() []index.Version {
	if !prepared.Available() {
		return nil
	}
	return append([]index.Version(nil), prepared.indexes...)
}

// SemanticChanged reports whether at least one semantic column changed.
func (prepared Prepared) SemanticChanged() bool { return prepared.Available() && prepared.semantic }

// LineageChanged reports whether at least one proof-only column changed.
func (prepared Prepared) LineageChanged() bool { return prepared.Available() && prepared.lineage }

// Empty reports the exact immutable no-op candidate.
func (prepared Prepared) Empty() bool { return prepared.Available() && prepared.noop }

// Commit is the sole aggregate publication operation. It validates the
// source candidate again and returns the already-built aggregate root and one
// delta; no index or column is published independently.
func Commit(prepared Prepared) (Version, Delta, bool) {
	if !prepared.Available() {
		return Version{}, Delta{}, false
	}
	if prepared.noop {
		return prepared.base, Delta{}, true
	}
	if prepared.source.Empty() {
		if prepared.contributionDelta == nil {
			return Version{}, Delta{}, false
		}
		delta := sealDelta(Delta{base: prepared.base, next: prepared.next, contribution: *prepared.contributionDelta, storeEmpty: true})
		if !delta.Available() {
			return Version{}, Delta{}, false
		}
		return prepared.next, delta, true
	}
	sourceDelta := prepared.source.Delta()
	if !sourceDelta.Available() || !sourceDelta.Next().Same(prepared.next.Store()) {
		return Version{}, Delta{}, false
	}
	deltaValue := Delta{base: prepared.base, next: prepared.next, source: sourceDelta, indexes: append([]index.Delta(nil), prepared.indexDeltas...), semantic: prepared.semantic, lineage: prepared.lineage}
	if prepared.contributionDelta != nil {
		deltaValue.contribution = *prepared.contributionDelta
	}
	delta := sealDelta(deltaValue)
	if !delta.Available() {
		return Version{}, Delta{}, false
	}
	return prepared.next, delta, true
}
