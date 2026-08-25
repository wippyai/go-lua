package lifecycle

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

// MountedSubjectLiveness is the typed, mount-aware result of redeeming one
// issued subject-liveness row. Its state, span, and mount are immutable
// capabilities borrowed from the authenticated publication and the mounted
// occurrence that issued the row.
type MountedSubjectLiveness struct {
	state programstate.State
	span  SubjectLivenessSpan
	mount identity.ContentID
}

// RedeemSubjectLiveness opens the lifecycle view privately, redeems the
// issued ordinal, and fences that row against the occurrence identity that
// issued it. The boundary at the span's lower endpoint must also be present;
// a span without its boundary is not a mounted candidate.
func RedeemSubjectLiveness(state programstate.State, ordinal uint32, mount, occurrence identity.ContentID) (MountedSubjectLiveness, bool) {
	if !state.Available() || !mount.Available() || !occurrence.Available() {
		return MountedSubjectLiveness{}, false
	}
	view, viewOK := NewView(state)
	if !viewOK {
		return MountedSubjectLiveness{}, false
	}
	span, spanOK := view.SubjectLivenessSpanAt(int(ordinal))
	if !spanOK || !span.Available() || span.ID() != occurrence {
		return MountedSubjectLiveness{}, false
	}
	boundary, boundaryOK := view.SubjectYieldBoundaryAt(int(span.Lo()))
	if !boundaryOK || !boundary.Available() || boundary.Ordinal() != span.Lo() {
		return MountedSubjectLiveness{}, false
	}
	candidate := MountedSubjectLiveness{
		state: state,
		span:  span,
		mount: mount,
	}
	if !candidate.Available() {
		return MountedSubjectLiveness{}, false
	}
	return candidate, true
}

// Available reports whether the redeemed candidate still carries every
// authenticated capability required by its owner relation.
func (candidate MountedSubjectLiveness) Available() bool {
	return candidate.state.Available() && candidate.span.Available() && candidate.mount.Available()
}

// State is the immutable publication from which the candidate was redeemed.
func (candidate MountedSubjectLiveness) State() programstate.State {
	if !candidate.Available() {
		return programstate.State{}
	}
	return candidate.state
}

// Span is the neutral liveness row redeemed at the issued ordinal.
func (candidate MountedSubjectLiveness) Span() SubjectLivenessSpan {
	if !candidate.Available() {
		return SubjectLivenessSpan{}
	}
	return candidate.span
}

// MountID is the authenticated mount that owns this candidate occurrence.
func (candidate MountedSubjectLiveness) MountID() identity.ContentID {
	if !candidate.Available() {
		return identity.ContentID{}
	}
	return candidate.mount
}

// StartBoundary exact-reads the canonical boundary at the span's lower
// endpoint. The boundary is deliberately not cached in the candidate: the
// lifecycle publication remains the sole authority for this derived row.
func (candidate MountedSubjectLiveness) StartBoundary() (SubjectYieldBoundary, bool) {
	if !candidate.Available() {
		return SubjectYieldBoundary{}, false
	}
	view, viewOK := NewView(candidate.state)
	if !viewOK {
		return SubjectYieldBoundary{}, false
	}
	boundary, boundaryOK := view.SubjectYieldBoundaryAt(int(candidate.span.Lo()))
	if !boundaryOK || !boundary.Available() || boundary.Ordinal() != candidate.span.Lo() {
		return SubjectYieldBoundary{}, false
	}
	return boundary, true
}

// BoundaryCallID is the canonical call occurrence at the span's lower
// boundary. It is derived from StartBoundary so no downstream identity mirror
// is retained in the candidate.
func (candidate MountedSubjectLiveness) BoundaryCallID() identity.ContentID {
	boundary, boundaryOK := candidate.StartBoundary()
	if !boundaryOK {
		return identity.ContentID{}
	}
	return boundary.CallID()
}
