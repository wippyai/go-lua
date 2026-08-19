package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestDiagnosticObservationQueryDoesNotExposeOutOfRangeEvidence(t *testing.T) {
	evidence, evidenceOK := programschema.NewDiagnosticEvidence(identity.ContentID{3})
	if !evidenceOK {
		t.Fatal("diagnostic evidence")
	}
	observation, observationOK := programschema.NewDiagnosticObservationBranchCondition(
		identity.ContentID{1},
		programsource.Span{File: "diagnostic.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2},
		0, 1, identity.ContentID{2}, identity.ContentID{4},
	)
	if !observationOK {
		t.Fatal("diagnostic observation")
	}
	schemaID := identity.ContentID{0xC0, 0x1D}
	catalog, catalogOK := programschema.CatalogID(schemaID)
	if !catalogOK {
		t.Fatal("diagnostic catalog")
	}
	frozen, sealed := (programschema.Publication{
		DiagnosticObservations: []programschema.DiagnosticObservation{observation},
		DiagnosticEvidence:     []programschema.DiagnosticEvidence{evidence},
	}).Seal(catalog, identity.StoreID(1))
	program := programschema.Program{
		Frozen: frozen, ArtifactID: identity.ContentID{0xA1},
		ProgramID: identity.ContentID{0xA2}, SchemaID: schemaID,
	}
	if !sealed || !program.Available() {
		t.Fatal("diagnostic program")
	}
	if point, ok := program.DiagnosticEvidencePointAt(0, 0); !ok || point != (identity.ContentID{3}) {
		t.Fatal("evidence point query lost its value")
	}
	if _, ok := program.DiagnosticEvidencePointAt(0, -1); ok {
		t.Fatal("negative evidence index was admitted")
	}
	if _, ok := program.DiagnosticEvidencePointAt(0, 1); ok {
		t.Fatal("evidence denominator was admitted")
	}
}
