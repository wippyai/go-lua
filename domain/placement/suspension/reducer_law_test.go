package suspension

import (
	"testing"

	reduceroperand "github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/analysis/schema/program/publication"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The two folds take the mounted candidate the Program row redeems into, the
// whole vector of the subject's Value cells, the route the selection issued,
// and the routed cell they publish into. The signatures are pinned here
// because the axis catalog derives its call shape from the declaration, and a
// judgment that drifts from that shape is a fold the composition can no longer
// invoke.
var (
	_ func(lifecycle.MountedSubjectLiveness, reduceroperand.SummaryVector[valuedomain.Value], heap.Key, uint64, placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) = SuspensionFold
	_ func(lifecycle.MountedSubjectLiveness, reduceroperand.SummaryVector[valuedomain.Value], heap.Key, uint64, Evidence) (Evidence, structure.ReductionOutcome)                         = SuspensionEvidenceFold
)

func suspensionReducerLawID(t testing.TB, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("placement-suspension-reducer-law", []byte(name))
	if !ok {
		t.Fatalf("derive %s identity", name)
	}
	return id
}

// suspensionReducerLawCandidate redeems one mounted subject-liveness row out
// of a sealed lifecycle publication, which is the only way a candidate of this
// fold comes to exist: the row is fenced against the occurrence that issued
// it and against the boundary at its own lower endpoint.
func suspensionReducerLawCandidate(t testing.TB, state lifecycle.SubjectLivenessState) lifecycle.MountedSubjectLiveness {
	t.Helper()
	schemaID := suspensionReducerLawID(t, "schema")
	catalogID, catalogOK := programcatalog.CatalogID(schemaID)
	if !catalogOK {
		t.Fatal("derive Program catalog")
	}
	call, route := suspensionReducerLawID(t, "call"), suspensionReducerLawID(t, "route")
	subject := suspensionReducerLawID(t, "subject")
	boundaryID, boundaryIDOK := lifecycle.SubjectYieldBoundaryIdentity(call, route)
	boundary, boundaryOK := lifecycle.NewSubjectYieldBoundary(boundaryID, call, route, identity.ContentID{}, identity.ContentID{}, 0)
	spanID, spanIDOK := lifecycle.SubjectLivenessSpanIdentity(lifecycle.SubjectLivenessCell, subject, 0, 0)
	span, spanOK := lifecycle.NewSubjectLivenessSpan(spanID, subject, lifecycle.SubjectLivenessCell, 0, 0, state)
	if !boundaryIDOK || !boundaryOK || !spanIDOK || !spanOK {
		t.Fatal("subject liveness law rows unavailable")
	}
	frozen, sealed := (publication.Publication{
		Lifecycle: lifecycle.Publication{
			SubjectSpans:      []lifecycle.SubjectLivenessSpan{span},
			SubjectBoundaries: []lifecycle.SubjectYieldBoundary{boundary},
		},
	}).Seal(catalogID, identity.StoreID(43))
	if !sealed {
		t.Fatal("seal lifecycle publication")
	}
	program := programschema.Program{
		Frozen: frozen, ArtifactID: suspensionReducerLawID(t, "artifact"),
		ProgramID: suspensionReducerLawID(t, "program"), SchemaID: schemaID,
	}
	state1, stateOK := program.ColdState()
	if !stateOK {
		t.Fatal("open cold Program state")
	}
	candidate, candidateOK := lifecycle.RedeemSubjectLiveness(state1, 0, suspensionReducerLawID(t, "mount"), spanID)
	if !candidateOK || !candidate.Available() {
		t.Fatalf("redeem mounted subject liveness for state %v", state)
	}
	return candidate
}

func suspensionReducerLawVector(t testing.TB, cells ...valuedomain.Value) reduceroperand.SummaryVector[valuedomain.Value] {
	t.Helper()
	members := make([]reduceroperand.MemberCell[valuedomain.Value], 0, len(cells))
	for _, cell := range cells {
		members = append(members, reduceroperand.MemberCell[valuedomain.Value]{Value: cell, Present: true})
	}
	vector, ok := reduceroperand.NewMemberVector(members)
	if !ok {
		t.Fatal("open source vector")
	}
	return vector
}

// A route this owner did not issue is not a destination. Every fold input is
// authenticated before any liveness consequence is applied, so an unissued
// route, a zero tag, an unredeemed candidate and an unauthenticated cell each
// refuse rather than publish a fact derived from evidence the fold cannot
// prove.
func TestSuspensionFoldsRefuseUnauthenticatedEvidence(t *testing.T) {
	candidate := suspensionReducerLawCandidate(t, lifecycle.SubjectLivenessLive)
	sources := suspensionReducerLawVector(t, valuedomain.Value{})

	if got, outcome := SuspensionFold(candidate, sources, heap.Key{}, 1, placementdomain.DefaultFact()); outcome != structure.Refuse || got != placementdomain.BottomFact() {
		t.Fatalf("unissued route fold=%v/%v, want Bottom/Refuse", got, outcome)
	}
	if got, outcome := SuspensionFold(candidate, sources, heap.Key{}, 0, placementdomain.DefaultFact()); outcome != structure.Refuse || got != placementdomain.BottomFact() {
		t.Fatalf("zero route tag fold=%v/%v, want Bottom/Refuse", got, outcome)
	}
	if got, outcome := SuspensionFold(lifecycle.MountedSubjectLiveness{}, sources, heap.Key{}, 1, placementdomain.DefaultFact()); outcome != structure.Refuse || got != placementdomain.BottomFact() {
		t.Fatalf("unredeemed candidate fold=%v/%v, want Bottom/Refuse", got, outcome)
	}
	if got, outcome := SuspensionFold(candidate, reduceroperand.SummaryVector[valuedomain.Value]{}, heap.Key{}, 1, placementdomain.DefaultFact()); outcome != structure.Refuse || got != placementdomain.BottomFact() {
		t.Fatalf("closed source vector fold=%v/%v, want Bottom/Refuse", got, outcome)
	}

	if got, outcome := SuspensionEvidenceFold(candidate, sources, heap.Key{}, 1, EvidenceProven); outcome != structure.Refuse || got != EvidenceMissing {
		t.Fatalf("unissued route evidence fold=%v/%v, want Missing/Refuse", got, outcome)
	}
	if got, outcome := SuspensionEvidenceFold(candidate, sources, heap.Key{}, 0, EvidenceProven); outcome != structure.Refuse || got != EvidenceMissing {
		t.Fatalf("zero route tag evidence fold=%v/%v, want Missing/Refuse", got, outcome)
	}
	if got, outcome := SuspensionEvidenceFold(lifecycle.MountedSubjectLiveness{}, sources, heap.Key{}, 1, EvidenceProven); outcome != structure.Refuse || got != EvidenceMissing {
		t.Fatalf("unredeemed candidate evidence fold=%v/%v, want Missing/Refuse", got, outcome)
	}
}

// The vector is folded whole: it is read as one span and a vector that is not
// open refuses, which is what makes the declared read a whole-vector delivery
// rather than a per-cell selection. The widening arm - one Top cell widening
// the conclusion - is decided by Value's own Top, which needs a sealed Value
// schema this package builds no fixture for; it is covered where that schema
// exists rather than mocked here.
func TestSuspensionSourceVectorIsReadAsOneSpan(t *testing.T) {
	known := suspensionReducerLawVector(t, valuedomain.Value{}, valuedomain.Value{})
	if widen, ok := sourceVectorUnknown(known); !ok || widen {
		t.Fatalf("known vector widen=%v/%v, want no widening", widen, ok)
	}
	if _, ok := sourceVectorUnknown(reduceroperand.SummaryVector[valuedomain.Value]{}); ok {
		t.Fatal("closed vector was read as a source vector")
	}
}
