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

func subjectLivenessValidationRow(t *testing.T, route, from, to, subject identity.ContentID, kind lifecycle.SubjectLivenessKind, state lifecycle.SubjectLivenessState) lifecycle.SubjectLiveness {
	t.Helper()
	id, ok := lifecycle.SubjectLivenessIdentity(route, kind, subject)
	if !ok {
		t.Fatal("subject-liveness identity")
	}
	row, ok := lifecycle.NewSubjectLiveness(id, route, from, to, subject, kind, state)
	if !ok {
		t.Fatal("subject-liveness row")
	}
	return row
}

func subjectLivenessValidationFixture(t *testing.T, rows []lifecycle.SubjectLiveness) *validator {
	t.Helper()
	catalog, ok := programcatalog.CatalogID(subjectLivenessValidationID(201))
	if !ok {
		t.Fatal("catalog")
	}
	frozen, ok := (Publication{Lifecycle: lifecycle.Publication{SubjectLifetimes: rows}}).Seal(catalog, identity.StoreID(3))
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

func TestSealValidationRejectsDuplicateSubjectLivenessRows(t *testing.T) {
	route := subjectLivenessValidationID(9)
	row := subjectLivenessValidationRow(
		t,
		route,
		subjectLivenessValidationID(41),
		subjectLivenessValidationID(73),
		subjectLivenessValidationID(105),
		lifecycle.SubjectLivenessValues,
		lifecycle.SubjectLivenessUnknown,
	)
	validator := subjectLivenessValidationFixture(t, []lifecycle.SubjectLiveness{row, row})
	if validator.validateSealRows(&validationState{}) {
		t.Fatal("seal validation admitted duplicate subject-liveness rows")
	}
}
