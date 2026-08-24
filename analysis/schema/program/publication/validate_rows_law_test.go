package publication

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

func subjectLivenessValidationID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func subjectLivenessValidationSpan(t *testing.T, subject identity.ContentID, kind lifecycle.SubjectLivenessKind, lo, hi uint32, state lifecycle.SubjectLivenessState) lifecycle.SubjectLivenessSpan {
	t.Helper()
	id, ok := lifecycle.SubjectLivenessSpanIdentity(kind, subject, lo, hi)
	if !ok {
		t.Fatal("subject-liveness span identity")
	}
	row, ok := lifecycle.NewSubjectLivenessSpan(id, subject, kind, lo, hi, state)
	if !ok {
		t.Fatal("subject-liveness span row")
	}
	return row
}

func subjectYieldBoundaryValidationRow(t *testing.T, route, from, to identity.ContentID, ordinal uint32) lifecycle.SubjectYieldBoundary {
	t.Helper()
	call := subjectLivenessValidationID(137)
	id, ok := lifecycle.SubjectYieldBoundaryIdentity(call, route)
	if !ok {
		t.Fatal("subject-yield-boundary identity")
	}
	row, ok := lifecycle.NewSubjectYieldBoundary(id, call, route, from, to, ordinal)
	if !ok {
		t.Fatal("subject-yield-boundary row")
	}
	return row
}

func subjectLivenessValidationFixture(t *testing.T, boundaries []lifecycle.SubjectYieldBoundary, spans []lifecycle.SubjectLivenessSpan) *validator {
	t.Helper()
	catalog, ok := programcatalog.CatalogID(subjectLivenessValidationID(201))
	if !ok {
		t.Fatal("catalog")
	}
	frozen, ok := (Publication{Lifecycle: lifecycle.Publication{SubjectBoundaries: boundaries, SubjectSpans: spans}}).Seal(catalog, identity.StoreID(3))
	if !ok {
		t.Fatal("publication seal")
	}
	state, ok := programstate.New(frozen, catalog)
	if !ok {
		t.Fatal("cold state")
	}
	lifecycleView, ok := lifecycle.NewView(state)
	if !ok {
		t.Fatal("lifecycle view")
	}
	return &validator{state: state, frozen: frozen, catalog: catalog, lifecycle: lifecycleView}
}

func TestSealValidationRejectsDuplicateSubjectLivenessSpans(t *testing.T) {
	span := subjectLivenessValidationSpan(
		t,
		subjectLivenessValidationID(105),
		lifecycle.SubjectLivenessValues,
		0, 0,
		lifecycle.SubjectLivenessUnknown,
	)
	validator := subjectLivenessValidationFixture(t, nil, []lifecycle.SubjectLivenessSpan{span, span})
	if validator.validateSealRows(&validationState{}) {
		t.Fatal("seal validation admitted duplicate subject-liveness spans")
	}
}

// Two boundaries at one ordinal would make every range that covers it
// ambiguous, so the seal refuses the numbering rather than the read.
func TestSealValidationRejectsDuplicateBoundaryOrdinals(t *testing.T) {
	first := subjectYieldBoundaryValidationRow(t, subjectLivenessValidationID(9), subjectLivenessValidationID(41), subjectLivenessValidationID(73), 0)
	second := subjectYieldBoundaryValidationRow(t, subjectLivenessValidationID(11), subjectLivenessValidationID(43), subjectLivenessValidationID(75), 0)
	validator := subjectLivenessValidationFixture(t, []lifecycle.SubjectYieldBoundary{first, second}, nil)
	if validator.validateSealRows(&validationState{}) {
		t.Fatal("seal validation admitted two boundaries at one ordinal")
	}
}
