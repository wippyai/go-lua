package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

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
				if row.Kind() != programartifact.DiagnosticObservationBranchCondition {
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
		if !rowOK || row.Kind() != programartifact.DiagnosticObservationTypeReferenceUnresolved {
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
		if rowOK && row.Kind() == programartifact.DiagnosticObservationTypeReferenceUnresolved {
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
		if !rowOK || row.Kind() != programartifact.DiagnosticObservationTypeReferenceUnresolved {
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
