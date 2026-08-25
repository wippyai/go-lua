package lifecycle

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func mountedSubjectLivenessLawRows(t *testing.T) (SubjectYieldBoundary, SubjectLivenessSpan, identity.ContentID) {
	t.Helper()
	call, route, subject := lifecycleLawID(t, "mounted-call"), lifecycleLawID(t, "mounted-route"), lifecycleLawID(t, "mounted-subject")
	boundaryID, boundaryIDOK := SubjectYieldBoundaryIdentity(call, route)
	boundary, boundaryOK := NewSubjectYieldBoundary(boundaryID, call, route, identity.ContentID{}, identity.ContentID{}, 0)
	spanID, spanIDOK := SubjectLivenessSpanIdentity(SubjectLivenessCell, subject, 0, 0)
	span, spanOK := NewSubjectLivenessSpan(spanID, subject, SubjectLivenessCell, 0, 0, SubjectLivenessLive)
	if !boundaryIDOK || !boundaryOK || !spanIDOK || !spanOK {
		t.Fatal("mounted liveness law rows unavailable")
	}
	return boundary, span, spanID
}

func TestRedeemSubjectLivenessCarriesAuthenticatedMountContext(t *testing.T) {
	boundary, span, occurrence := mountedSubjectLivenessLawRows(t)
	catalog := lifecycleLawID(t, "mounted-catalog")
	state := lifecycleLawView(t, Publication{
		SubjectBoundaries: []SubjectYieldBoundary{boundary},
		SubjectSpans:      []SubjectLivenessSpan{span},
	}, catalog).State()
	mount := lifecycleLawID(t, "mounted-mount")
	candidate, candidateOK := RedeemSubjectLiveness(state, 0, mount, occurrence)
	if !candidateOK || !candidate.Available() {
		t.Fatal("valid mounted liveness row was refused")
	}
	start, startOK := candidate.StartBoundary()
	if !startOK || start.ID() != boundary.ID() || candidate.State().CatalogID() != catalog || candidate.Span().ID() != occurrence || candidate.MountID() != mount || candidate.BoundaryCallID() != boundary.CallID() {
		t.Fatal("mounted liveness candidate lost neutral capabilities")
	}
}

func TestRedeemSubjectLivenessRefusesMismatchedOrUnboundContext(t *testing.T) {
	boundary, span, occurrence := mountedSubjectLivenessLawRows(t)
	catalog := lifecycleLawID(t, "mounted-negative-catalog")
	state := lifecycleLawView(t, Publication{
		SubjectBoundaries: []SubjectYieldBoundary{boundary},
		SubjectSpans:      []SubjectLivenessSpan{span},
	}, catalog).State()
	mount := lifecycleLawID(t, "mounted-negative-mount")
	if _, ok := RedeemSubjectLiveness(state, 0, identity.ContentID{}, occurrence); ok {
		t.Fatal("mounted liveness row admitted without a mount")
	}
	if _, ok := RedeemSubjectLiveness(state, 0, mount, lifecycleLawID(t, "mounted-wrong-occurrence")); ok {
		t.Fatal("mounted liveness row admitted a mismatched occurrence")
	}
	noBoundaryState := lifecycleLawView(t, Publication{SubjectSpans: []SubjectLivenessSpan{span}}, lifecycleLawID(t, "mounted-no-boundary")).State()
	if _, ok := RedeemSubjectLiveness(noBoundaryState, 0, mount, occurrence); ok {
		t.Fatal("mounted liveness row admitted without its lower boundary")
	}
}
