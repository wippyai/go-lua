package artifact_test

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

func TestProgramArtifactRetainsUnresolvedValueCandidateWithoutRuntimeGeometry(t *testing.T) {
	left := compileStaticReferenceLeafArtifact(t, "unresolved-value-artifact.lua", `
local total = missing_count + 1
return total
`)
	right := compileStaticReferenceLeafArtifact(t, "unresolved-value-artifact.lua", `
local total = missing_count + 1
return total
`)
	if left.DiagnosticObservationCount() != 1 || right.DiagnosticObservationCount() != 1 {
		t.Fatalf("diagnostic observation count = %d/%d, want 1/1", left.DiagnosticObservationCount(), right.DiagnosticObservationCount())
	}
	row, rowOK := left.DiagnosticObservationAt(0)
	replayed, replayedOK := right.DiagnosticObservationAt(0)
	payload, payloadOK := row.UnresolvedValueReference()
	replayedPayload, replayedPayloadOK := replayed.UnresolvedValueReference()
	name, nameOK := payload.Name()
	replayedName, replayedNameOK := replayedPayload.Name()
	location, locationOK := row.Location()
	if !rowOK || !replayedOK || row.Kind() != programartifact.DiagnosticObservationValueReferenceUnresolved || replayed.Kind() != row.Kind() ||
		!payloadOK || !replayedPayloadOK || !nameOK || !replayedNameOK || name != "missing_count" || replayedName != name ||
		!payload.ReadID().Available() || !payload.CellID().Available() || payload.ReadID() == payload.CellID() ||
		payload.ReadID() != replayedPayload.ReadID() || payload.CellID() != replayedPayload.CellID() || row.ID() != replayed.ID() ||
		!locationOK || location.File != "unresolved-value-artifact.lua" || location.StartLine != 2 || location.StartCol != 15 {
		t.Fatalf("unresolved value artifact row = row:%v/%v kind:%d/%d payload:%v/%v name:%q/%q location:%+v/%v", rowOK, replayedOK, row.Kind(), replayed.Kind(), payloadOK, replayedPayloadOK, name, replayedName, location, locationOK)
	}
	if _, ok := left.DiagnosticObservationAt(-1); ok {
		t.Fatal("DiagnosticObservationAt accepted a negative index")
	}
	if _, ok := left.DiagnosticObservationAt(left.DiagnosticObservationCount()); ok {
		t.Fatal("DiagnosticObservationAt accepted its denominator")
	}
}
