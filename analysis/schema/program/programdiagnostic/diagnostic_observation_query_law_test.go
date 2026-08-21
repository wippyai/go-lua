package programdiagnostic_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

func TestDiagnosticObservationQueryDoesNotExposeOutOfRangeEvidenceLaw(t *testing.T) {
	evidence, evidenceOK := programdiagnostic.NewDiagnosticEvidence(identity.ContentID{3})
	if !evidenceOK {
		t.Fatal("diagnostic evidence")
	}
	observation, observationOK := programdiagnostic.NewDiagnosticObservationBranchCondition(
		identity.ContentID{1},
		programsource.Span{File: "diagnostic.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2},
		0, 1, identity.ContentID{2}, identity.ContentID{4},
	)
	if !observationOK {
		t.Fatal("diagnostic observation")
	}
	schemaID := identity.ContentID{0xC0, 0x1D}
	catalog, catalogOK := programcatalog.CatalogID(schemaID)
	if !catalogOK {
		t.Fatal("diagnostic catalog")
	}
	frozen, sealed := (programpublication.Publication{
		Diagnostic: programdiagnostic.Publication{
			DiagnosticObservations: []programdiagnostic.DiagnosticObservation{observation},
			DiagnosticEvidence:     []programdiagnostic.DiagnosticEvidence{evidence},
		},
	}).Seal(catalog, identity.StoreID(1))
	state, stateOK := programstate.New(frozen, catalog)
	view, viewOK := programdiagnostic.NewView(state)
	if !sealed || !stateOK || !viewOK {
		t.Fatal("diagnostic program")
	}
	if point, ok := view.DiagnosticEvidencePointAt(0, 0); !ok || point != (identity.ContentID{3}) {
		t.Fatal("evidence point query lost its value")
	}
	if _, ok := view.DiagnosticEvidencePointAt(0, -1); ok {
		t.Fatal("negative evidence index was admitted")
	}
	if _, ok := view.DiagnosticEvidencePointAt(0, 1); ok {
		t.Fatal("evidence denominator was admitted")
	}
}
