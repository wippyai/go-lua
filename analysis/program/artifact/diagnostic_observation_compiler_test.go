package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func TestProgramArtifactDiagnosticCompilerKeepsBranchObservationIDsStable(t *testing.T) {
	left := compileStaticReferenceLeafArtifact(t, "diagnostic-compiler.lua", "local flag = true\nif flag then flag = false end\nreturn flag\n")
	right := compileStaticReferenceLeafArtifact(t, "diagnostic-compiler.lua", "local flag = true\nif flag then flag = false end\nreturn flag\n")
	leftIDs, rightIDs := make(map[identity.ContentID]struct{}), make(map[identity.ContentID]struct{})
	for index := 0; index < left.DiagnosticObservationCount(); index++ {
		row, ok := left.DiagnosticObservationAt(index)
		if ok && row.Kind() == structure.DiagnosticObservationBranchCondition {
			leftIDs[row.ID()] = struct{}{}
		}
	}
	for index := 0; index < right.DiagnosticObservationCount(); index++ {
		row, ok := right.DiagnosticObservationAt(index)
		if ok && row.Kind() == structure.DiagnosticObservationBranchCondition {
			rightIDs[row.ID()] = struct{}{}
		}
	}
	if len(leftIDs) == 0 || len(leftIDs) != len(rightIDs) {
		t.Fatalf("branch observation IDs = %d/%d", len(leftIDs), len(rightIDs))
	}
	for id := range leftIDs {
		if _, ok := rightIDs[id]; !ok {
			t.Fatal("diagnostic compiler changed a stable branch observation ID")
		}
	}
}
func TestProgramArtifactBranchDiagnosticRequiresScopePreservingRewrite(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantUnique int
	}{
		{
			name:       "assignment-only",
			source:     "local flag = true\nif flag then\n  flag = false\nend\nreturn flag\n",
			wantUnique: 1,
		},
		{
			name:       "local-introduction",
			source:     "if true then\n  local scoped = 1\nend\nreturn 0\n",
			wantUnique: 0,
		},
		{
			name:       "static-type-introduction",
			source:     "if true then\n  type Scoped = {x: number}\nend\nreturn 0\n",
			wantUnique: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact := compileStaticReferenceLeafArtifact(t, "branch-diagnostic-scope-artifact.lua", test.source)
			unique := make(map[identity.ContentID]struct{})
			for index := 0; index < artifact.DiagnosticObservationCount(); index++ {
				row, rowOK := artifact.DiagnosticObservationAt(index)
				if !rowOK {
					t.Fatalf("DiagnosticObservationAt(%d)", index)
				}
				if row.Kind() != structure.DiagnosticObservationBranchCondition {
					continue
				}
				if _, payloadOK := row.BranchCondition(); !payloadOK || !row.ID().Available() {
					t.Fatal("issued branch diagnostic row is malformed")
				}
				unique[row.ID()] = struct{}{}
			}
			if len(unique) != test.wantUnique {
				t.Fatalf("unique branch observations = %d, want %d", len(unique), test.wantUnique)
			}
		})
	}
}

func TestProgramArtifactUnresolvedTypeObservationCarriesExactStaticProof(t *testing.T) {
	left := compileStaticReferenceLeafArtifact(t, "diagnostic-observation-artifact.lua", "type MissingAlias = Missing\n")
	right := compileStaticReferenceLeafArtifact(t, "diagnostic-observation-artifact.lua", "type MissingAlias = Missing\n")
	var leftID, rightID identity.ContentID
	for index := 0; index < left.DiagnosticObservationCount(); index++ {
		row, rowOK := left.DiagnosticObservationAt(index)
		if !rowOK || row.Kind() != structure.DiagnosticObservationTypeReferenceUnresolved {
			continue
		}
		payload, payloadOK := row.UnresolvedTypeReference()
		path, pathOK := payload.Path()
		location, locationOK := row.Location()
		if !payloadOK || !pathOK || len(path) != 1 || path[0] != "Missing" || !locationOK || location.File != "diagnostic-observation-artifact.lua" {
			t.Fatalf("unresolved payload = %#v/%v path=%v/%v location=%#v/%v", payload, payloadOK, path, pathOK, location, locationOK)
		}
		leftID = row.ID()
	}
	for index := 0; index < right.DiagnosticObservationCount(); index++ {
		row, rowOK := right.DiagnosticObservationAt(index)
		if rowOK && row.Kind() == structure.DiagnosticObservationTypeReferenceUnresolved {
			rightID = row.ID()
		}
	}
	if !leftID.Available() || !rightID.Available() || leftID != rightID {
		t.Fatalf("deterministic unresolved observation = %x/%x", leftID, rightID)
	}
}

func TestProgramArtifactQualifiedUnresolvedTypeObservationCarriesRootProof(t *testing.T) {
	artifact := compileStaticReferenceLeafArtifact(t, "qualified-diagnostic-observation-artifact.lua", "type MissingAlias = Missing.Namespace\n")
	for index := 0; index < artifact.DiagnosticObservationCount(); index++ {
		row, rowOK := artifact.DiagnosticObservationAt(index)
		if !rowOK || row.Kind() != structure.DiagnosticObservationTypeReferenceUnresolved {
			continue
		}
		payload, payloadOK := row.UnresolvedTypeReference()
		path, pathOK := payload.Path()
		if payloadOK && pathOK && len(path) == 2 && path[0] == "Missing" && path[1] == "Namespace" && payload.RootID().Available() {
			return
		}
		t.Fatalf("qualified unresolved type row = payload:%v/%v path:%v/%v root:%v", payload, payloadOK, path, pathOK, payload.RootID())
	}
	t.Fatal("qualified unresolved type reference was not issued")
}

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
	if !rowOK || !replayedOK || row.Kind() != structure.DiagnosticObservationValueReferenceUnresolved || replayed.Kind() != row.Kind() ||
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
