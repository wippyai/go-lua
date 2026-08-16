package programartifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/programartifact"
)

func TestProgramArtifactRetainsUnresolvedValueCandidateWithoutRuntimeGeometry(t *testing.T) {
	artifact := compileStaticReferenceLeafArtifact(t, "unresolved-value-artifact.lua", `
local total = missing_count + 1
return total
`)
	if artifact.DiagnosticObservationCount() != 1 {
		t.Fatalf("diagnostic observation count = %d, want 1", artifact.DiagnosticObservationCount())
	}
	row, rowOK := artifact.DiagnosticObservationAt(0)
	payload, payloadOK := row.UnresolvedValueReference()
	name, nameOK := payload.Name()
	location, locationOK := row.Location()
	if !rowOK || row.Kind() != programartifact.DiagnosticObservationValueReferenceUnresolved || !payloadOK || !nameOK ||
		name != "missing_count" || !payload.ReadID().Available() || !payload.CellID().Available() || payload.ReadID() == payload.CellID() ||
		!locationOK || location.File != "unresolved-value-artifact.lua" || location.StartLine != 2 || location.StartCol != 15 {
		t.Fatalf("unresolved value artifact row = row:%v kind:%d payload:%v name:%q location:%+v/%v", rowOK, row.Kind(), payloadOK, name, location, locationOK)
	}
}
