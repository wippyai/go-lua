package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func diagnosticProgram(t *testing.T, artifact *programartifact.Artifact) programschema.Program {
	t.Helper()
	program := artifact.Program()
	if !program.Available() {
		t.Fatal("diagnostic program unavailable")
	}
	return program
}

func TestProgramArtifactDiagnosticCompilerKeepsBranchObservationIDsStable(t *testing.T) {
	left := compileStaticReferenceLeafArtifact(t, "diagnostic-compiler.lua", "local flag = true\nif flag then flag = false end\nreturn flag\n")
	right := compileStaticReferenceLeafArtifact(t, "diagnostic-compiler.lua", "local flag = true\nif flag then flag = false end\nreturn flag\n")
	leftProgram, rightProgram := diagnosticProgram(t, left), diagnosticProgram(t, right)
	leftIDs, rightIDs := make(map[identity.ContentID]struct{}), make(map[identity.ContentID]struct{})
	leftCount, leftPublished := leftProgram.DiagnosticObservationCount()
	rightCount, rightPublished := rightProgram.DiagnosticObservationCount()
	if !leftPublished || !rightPublished {
		t.Fatal("diagnostic observation family unavailable")
	}
	for index := 0; index < leftCount; index++ {
		row, ok := leftProgram.DiagnosticObservationAt(index)
		if ok && row.Kind() == structure.DiagnosticObservationBranchCondition {
			leftIDs[row.ID()] = struct{}{}
		}
	}
	for index := 0; index < rightCount; index++ {
		row, ok := rightProgram.DiagnosticObservationAt(index)
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
			program := diagnosticProgram(t, artifact)
			count, published := program.DiagnosticObservationCount()
			if !published {
				t.Fatal("diagnostic observation family unavailable")
			}
			unique := make(map[identity.ContentID]struct{})
			for index := 0; index < count; index++ {
				row, rowOK := program.DiagnosticObservationAt(index)
				if !rowOK {
					t.Fatalf("DiagnosticObservationAt(%d)", index)
				}
				if row.Kind() != structure.DiagnosticObservationBranchCondition {
					continue
				}
				if !row.DecisionPathID().Available() || !row.ValueSpanID().Available() || !row.ID().Available() {
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
	leftProgram, rightProgram := diagnosticProgram(t, left), diagnosticProgram(t, right)
	var leftID, rightID identity.ContentID
	leftCount, leftPublished := leftProgram.DiagnosticObservationCount()
	rightCount, rightPublished := rightProgram.DiagnosticObservationCount()
	if !leftPublished || !rightPublished {
		t.Fatal("diagnostic observation family unavailable")
	}
	for index := 0; index < leftCount; index++ {
		row, rowOK := leftProgram.DiagnosticObservationAt(index)
		if !rowOK || row.Kind() != structure.DiagnosticObservationTypeReferenceUnresolved {
			continue
		}
		path, pathOK := diagnosticPath(leftProgram, index)
		location, locationOK := row.Location()
		if !pathOK || len(path) != 1 || path[0] != "Missing" || !locationOK || location.File != "diagnostic-observation-artifact.lua" {
			t.Fatalf("unresolved payload path=%v/%v location=%#v/%v", path, pathOK, location, locationOK)
		}
		leftID = row.ID()
	}
	for index := 0; index < rightCount; index++ {
		row, rowOK := rightProgram.DiagnosticObservationAt(index)
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
	program := diagnosticProgram(t, artifact)
	count, published := program.DiagnosticObservationCount()
	if !published {
		t.Fatal("diagnostic observation family unavailable")
	}
	for index := 0; index < count; index++ {
		row, rowOK := program.DiagnosticObservationAt(index)
		if !rowOK || row.Kind() != structure.DiagnosticObservationTypeReferenceUnresolved {
			continue
		}
		path, pathOK := diagnosticPath(program, index)
		if pathOK && len(path) == 2 && path[0] == "Missing" && path[1] == "Namespace" && row.RootID().Available() {
			return
		}
		t.Fatalf("qualified unresolved type row = path:%v/%v root:%v", path, pathOK, row.RootID())
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
	leftProgram, rightProgram := diagnosticProgram(t, left), diagnosticProgram(t, right)
	leftCount, leftPublished := leftProgram.DiagnosticObservationCount()
	rightCount, rightPublished := rightProgram.DiagnosticObservationCount()
	if !leftPublished || !rightPublished || leftCount != 1 || rightCount != 1 {
		t.Fatalf("diagnostic observation count = %d/%d, want 1/1", leftCount, rightCount)
	}
	row, rowOK := leftProgram.DiagnosticObservationAt(0)
	replayed, replayedOK := rightProgram.DiagnosticObservationAt(0)
	name, nameOK := row.Name(), row.Name() != ""
	replayedName, replayedNameOK := replayed.Name(), replayed.Name() != ""
	location, locationOK := row.Location()
	if !rowOK || !replayedOK || row.Kind() != structure.DiagnosticObservationValueReferenceUnresolved || replayed.Kind() != row.Kind() ||
		!nameOK || !replayedNameOK || name != "missing_count" || replayedName != name ||
		!row.ReadID().Available() || !row.CellID().Available() || row.ReadID() == row.CellID() ||
		row.ReadID() != replayed.ReadID() || row.CellID() != replayed.CellID() || row.ID() != replayed.ID() ||
		!locationOK || location.File != "unresolved-value-artifact.lua" || location.StartLine != 2 || location.StartCol != 15 {
		t.Fatalf("unresolved value artifact row = row:%v/%v kind:%d/%d name:%q/%q location:%+v/%v", rowOK, replayedOK, row.Kind(), replayed.Kind(), name, replayedName, location, locationOK)
	}
	if _, ok := leftProgram.DiagnosticObservationAt(-1); ok {
		t.Fatal("DiagnosticObservationAt accepted a negative index")
	}
	if _, ok := leftProgram.DiagnosticObservationAt(leftCount); ok {
		t.Fatal("DiagnosticObservationAt accepted its denominator")
	}
}

func TestSelectedDirectCallArgumentIssuesTypeConformance(t *testing.T) {
	artifact := compileStaticReferenceLeafArtifact(t, "conformance-call.lua", `
local function dormant(value: number)
  return identity(value)
end
local function identity(value: number)
  return value
end
return identity(1)
`)
	program := diagnosticProgram(t, artifact)
	count, published := program.DiagnosticObservationCount()
	if !published {
		t.Fatal("diagnostic observation family unavailable")
	}
	var rows int
	for index := 0; index < count; index++ {
		row, rowOK := program.DiagnosticObservationAt(index)
		if !rowOK {
			t.Fatalf("DiagnosticObservationAt(%d)", index)
		}
		if row.Kind() != structure.DiagnosticObservationTypeConformance {
			continue
		}
		rows++
		location, locationOK := row.Location()
		position, positionOK := row.Position()
		if !locationOK || !positionOK || position != 0 ||
			!row.OwnerID().Available() || !row.MeasuredValueID().Available() ||
			!row.DeclaredStaticTypeID().Available() || location.File != "conformance-call.lua" {
			t.Fatalf("selected call-argument observation is incomplete: location=%+v", location)
		}
	}
	if rows != 1 {
		t.Fatalf("type-conformance rows = %d, want 1 (the selected identity(1) argument)", rows)
	}
}

// TestDeclaredBindAndWriteIssueAssignmentConformance states the assignment half
// of the conformance relation. A cell authored with a declared type is measured
// wherever a value is written into it - the initializer that binds it and every
// later write - and an undeclared cell is measured nowhere, so the population is
// the declaration's, not the statement's.
func TestDeclaredBindAndWriteIssueAssignmentConformance(t *testing.T) {
	artifact := compileStaticReferenceLeafArtifact(t, "conformance-assign.lua", `
local declared: number = 1
declared = 2
local inferred = 3
inferred = 4
return declared + inferred
`)
	program := diagnosticProgram(t, artifact)
	count, published := program.DiagnosticObservationCount()
	if !published {
		t.Fatal("diagnostic observation family unavailable")
	}
	sites := make(map[uint32]int)
	for index := 0; index < count; index++ {
		row, rowOK := program.DiagnosticObservationAt(index)
		if !rowOK {
			t.Fatalf("DiagnosticObservationAt(%d)", index)
		}
		if row.Kind() != structure.DiagnosticObservationTypeConformance {
			continue
		}
		location, locationOK := row.Location()
		if !locationOK || location.File != "conformance-assign.lua" ||
			!row.OwnerID().Available() || !row.MeasuredValueID().Available() || !row.DeclaredStaticTypeID().Available() {
			t.Fatalf("assignment observation is incomplete: location=%+v", location)
		}
		if row.Site() != programschema.DiagnosticObservationSiteAssignment {
			t.Fatalf("observation at %d:%d carries site %v", location.StartLine, location.StartCol, row.Site())
		}
		sites[location.StartLine]++
	}
	if len(sites) != 2 || sites[2] != 1 || sites[3] != 1 {
		t.Fatalf("assignment conformance rows by line = %v, want one at the declared bind and one at its write", sites)
	}
}

func diagnosticPath(program programschema.Program, index int) ([]string, bool) {
	row, held := program.DiagnosticObservationAt(index)
	if !held {
		return nil, false
	}
	offset, count, spanOK := row.PathSpan()
	if !spanOK || count == 0 {
		return nil, false
	}
	path := make([]string, count)
	for childIndex := uint32(0); childIndex < count; childIndex++ {
		child, childOK := program.DiagnosticPathAt(int(offset + childIndex))
		if !childOK {
			return nil, false
		}
		path[childIndex] = child.Component()
	}
	return path, true
}
