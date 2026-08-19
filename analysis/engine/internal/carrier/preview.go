package carrier

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// previewOwner is the linear lifetime token carried by a temporary State.
// Keeping it on State, rather than maintaining a parallel State vocabulary,
// lets existing typed Binding reads and staged writes operate on exactly the
// same carrier representation.  Ordinary publication rejects such a State.
type previewOwner struct {
	work  *Work
	live  bool
	roots []previewRoot
}

// previewRoot binds the one temporary handle to both its physical slot and
// the Factor publisher that proves its typed plane remains live. It is owned
// solely by Preview and never enters State/SlotOperation vocabulary.
type previewRoot struct {
	slot      shape.Slot
	handle    RootHandle
	publisher PreviewRootPublisher
}

func (owner *previewOwner) owns(slot shape.Slot, handle RootHandle) bool {
	if owner == nil || !owner.live || !isPreviewRoot(handle) {
		return false
	}
	for index := len(owner.roots) - 1; index >= 0; index-- {
		root := owner.roots[index]
		if root.slot == slot && sameRoot(root.handle, handle) && root.publisher != nil && root.publisher.OwnsPreviewRoot(handle) {
			return true
		}
	}
	return false
}

// Preview is one evaluator-local, non-publishing candidate transaction.  It
// shares the normal State, Patch, View, and typed SlotWork semantics; only the
// final prepared Factor roots remain Binding-owned temporary planes until
// Abort.  It is intentionally not a second worklist, solver, or carrier.
type Preview struct {
	work  *Work
	owner *previewOwner
	state State
}

func (state State) previewMarked() bool {
	return state.authority != nil && state.authority.preview != nil
}

func (state State) previewOwner() *previewOwner {
	if state.authority == nil {
		return nil
	}
	return state.authority.preview
}

// BeginPreview starts a single linear candidate from one committed State.
// The caller must Abort it even after a successful comparison; no preview
// state may become a normal State or enter the root store.
func (work *Work) BeginPreview(base State) (*Preview, bool) {
	if !work.live() || work.previewing || base.previewMarked() || base.contributionMarked() || !work.OwnsState(base) {
		return nil, false
	}
	owner := &previewOwner{work: work, live: true}
	work.previewing = true
	state := State{authority: &stateAuthority{composition: base.authority.composition, epoch: work.epoch, preview: owner}, scope: base.scope, support: base.support, roots: base.roots}
	return &Preview{work: work, owner: owner, state: state}, true
}

// OwnsPreview proves that Preview is this exact evaluator's one live linear
// transaction. States alone identify a sealed Composition, not its mutable
// Work; callers that accept a Preview must make this stronger check before
// using its temporary roots.
func (work *Work) OwnsPreview(preview *Preview) bool {
	return work != nil && preview != nil && preview.work == work && preview.owner != nil && preview.owner.work == work && preview.live()
}

// State returns the current temporary State for typed reads and staged writes.
// It is valid only until Abort and is rejected by every ordinary publication
// operation.
func (preview *Preview) State() State {
	if !preview.live() {
		return State{}
	}
	return preview.state
}

// Restrict returns a normal support-only View of the current temporary State.
func (preview *Preview) Restrict(within support.Mask) (View, bool) {
	if !preview.live() {
		return View{}, false
	}
	return preview.state.Restrict(within)
}

// Commit applies one direct staged batch to the current candidate using the
// exact same common-predecessor and typed-region admission as Work.Commit.
func (preview *Preview) Commit(patches []Patch) (State, ChangeSet, bool) {
	if !preview.live() {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	empty := emptyMask(preview.work.composition.guards)
	if !empty.Valid() {
		dropPatches(patches)
		preview.Abort()
		return State{}, ChangeSet{}, false
	}
	return preview.apply(patches, preview.state.support, empty, empty, nil)
}

// Transfer applies a serial From group to the current temporary State.  The
// restricted View must be one of that exact candidate, so later groups read
// roots produced by earlier groups without publishing any of them.
func (preview *Preview) Transfer(restricted View, patches []Patch) (State, ChangeSet, bool) {
	if !preview.live() || !preview.work.OwnsViewOf(preview.state, restricted) {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	split, ok := preview.work.threeSupport(preview.state.support, restricted.support)
	if !ok {
		dropPatches(patches)
		preview.Abort()
		return State{}, ChangeSet{}, false
	}
	empty := emptyMask(preview.work.composition.guards)
	removed := split.LeftOnly()
	if !empty.Valid() || !removed.Valid() {
		dropPatches(patches)
		preview.Abort()
		return State{}, ChangeSet{}, false
	}
	return preview.apply(patches, restricted.support, empty, removed, nil)
}

// LessOrEq compares the final candidate with a committed converged State
// using the ordinary typed carrier order over the candidate's exact support.
func (preview *Preview) LessOrEq(existing State) bool {
	return preview.live() && !existing.previewMarked() && !existing.contributionMarked() && preview.work.LessOrEqUnder(preview.state, existing)
}

// Abort revokes every temporary typed plane and invalidates all escaped
// preview States/Views.  It never reserves or publishes a root-store entry.
func (preview *Preview) Abort() bool {
	if !preview.live() {
		return false
	}
	for _, root := range preview.owner.roots {
		root.publisher.Drop()
	}
	clear(preview.owner.roots)
	preview.owner.roots = nil
	preview.owner.live = false
	preview.work.previewing = false
	preview.state = State{}
	return true
}

func (preview *Preview) live() bool {
	return preview != nil && preview.work != nil && preview.owner != nil && preview.owner.work == preview.work && preview.owner.live && preview.work.previewing && preview.state.previewOwner() == preview.owner && preview.state.live()
}

func (preview *Preview) apply(patches []Patch, nextSupport, added, removed support.Mask, candidate *support.Work) (State, ChangeSet, bool) {
	if !preview.work.live() {
		dropPatches(patches)
		discardCandidate(candidate)
		preview.Abort()
		return State{}, ChangeSet{}, false
	}
	prepared, ok := preview.work.prepareCommit(preview.state, patches, nextSupport, added, removed, candidate)
	if !ok {
		dropPatches(patches)
		discardCandidate(candidate)
		preview.Abort()
		return State{}, ChangeSet{}, false
	}
	if !prepared.changed {
		dropPatches(patches)
		discardCandidate(candidate)
		return preview.state, prepared.set, true
	}
	var next []RootHandle
	if prepared.rootsChanged {
		next = append([]RootHandle(nil), preview.state.roots...)
	} else {
		next = preview.state.roots
	}
	held := make([]previewRoot, 0, len(patches))
	for _, patch := range patches {
		if !preview.work.live() {
			preview.dropAttempt(held, patches, candidate)
			return State{}, ChangeSet{}, false
		}
		record := patch.change.record
		if publisher := record.publisher; publisher != nil {
			previewPublisher, supported := publisher.(PreviewRootPublisher)
			if !supported {
				preview.dropAttempt(held, patches, candidate)
				return State{}, ChangeSet{}, false
			}
			root, valid := previewPublisher.PreviewRoot()
			if !valid || !previewPublisher.OwnsPreviewRoot(root) {
				preview.dropAttempt(held, patches, candidate)
				return State{}, ChangeSet{}, false
			}
			next[int(patch.slot)] = root
			held = append(held, previewRoot{slot: patch.slot, handle: root, publisher: previewPublisher})
		} else if prepared.rootsChanged {
			next[int(patch.slot)] = record.after
		}
	}
	if candidate != nil && !candidate.Seal() {
		preview.dropAttempt(held, patches, candidate)
		return State{}, ChangeSet{}, false
	}
	if !preview.work.live() {
		preview.dropAttempt(held, patches, candidate)
		return State{}, ChangeSet{}, false
	}
	if prepared.rootsChanged && preview.state.support.SameHandle(nextSupport) {
		prepared.set.set.Direction |= preview.work.rootMoveDirection(preview.state.roots, next, patches, nextSupport)
	}
	for _, patch := range patches {
		patch.change.record.consumed = true
	}
	preview.owner.roots = append(preview.owner.roots, held...)
	preview.state = State{authority: preview.state.authority, scope: preview.state.scope, support: nextSupport, roots: next}
	return preview.state, prepared.set, true
}

func (preview *Preview) dropAttempt(held []previewRoot, patches []Patch, candidate *support.Work) {
	for _, root := range held {
		root.publisher.Drop()
	}
	dropPatches(patches)
	discardCandidate(candidate)
	preview.Abort()
}
