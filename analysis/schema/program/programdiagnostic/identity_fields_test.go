package programdiagnostic

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

type identityReplay struct{ fields []string }

func (replay *identityReplay) WriteContentID(value identity.ContentID) bool {
	replay.fields = append(replay.fields, fmt.Sprintf("id:%x", value))
	return true
}
func (replay *identityReplay) WriteUint(value uint64) bool {
	replay.fields = append(replay.fields, fmt.Sprintf("uint:%d", value))
	return true
}
func (replay *identityReplay) WriteBool(value bool) bool {
	replay.fields = append(replay.fields, fmt.Sprintf("bool:%t", value))
	return true
}
func (replay *identityReplay) WriteString(value string) bool {
	replay.fields = append(replay.fields, "string:"+value)
	return true
}

func TestWriteArtifactIdentityFieldsReplaysEveryDiagnosticPayload(t *testing.T) {
	span := programsource.Span{File: "identity.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	branch, branchOK := NewDiagnosticObservationBranchCondition(programDiagnosticLawID(t, "branch"), span, 0, 1, programDiagnosticLawID(t, "decision"), programDiagnosticLawID(t, "value"))
	typeReference, typeReferenceOK := NewDiagnosticObservationTypeReferenceUnresolved(programDiagnosticLawID(t, "type-reference"), span, 0, 2, programDiagnosticLawID(t, "reference"), programDiagnosticLawID(t, "root"))
	valueReference, valueReferenceOK := NewDiagnosticObservationValueReferenceUnresolved(programDiagnosticLawID(t, "value-reference"), span, programDiagnosticLawID(t, "read"), programDiagnosticLawID(t, "cell"), "missing")
	conformance, conformanceOK := NewDiagnosticObservationTypeConformance(programDiagnosticLawID(t, "conformance"), span, 1, 1, DiagnosticObservationSiteAssignment, programDiagnosticLawID(t, "owner"), programDiagnosticLawID(t, "measured"), programDiagnosticLawID(t, "declared"), programDiagnosticLawID(t, "span"), 3, "opened.id")
	if !branchOK || !typeReferenceOK || !valueReferenceOK || !conformanceOK {
		t.Fatal("diagnostic fixture construction failed")
	}
	evidence0, evidence0OK := NewDiagnosticEvidence(programDiagnosticLawID(t, "evidence-0"))
	evidence1, evidence1OK := NewDiagnosticEvidence(programDiagnosticLawID(t, "evidence-1"))
	path0, path0OK := NewDiagnosticPath("pkg")
	path1, path1OK := NewDiagnosticPath("name")
	if !evidence0OK || !evidence1OK || !path0OK || !path1OK {
		t.Fatal("diagnostic child fixture construction failed")
	}
	view := programDiagnosticLawView(t, Publication{
		DiagnosticObservations: []DiagnosticObservation{branch, typeReference, valueReference, conformance},
		DiagnosticEvidence:     []DiagnosticEvidence{evidence0, evidence1},
		DiagnosticPaths:        []DiagnosticPath{path0, path1},
	}, programDiagnosticLawID(t, "catalog"))
	var first, second identityReplay
	if !view.WriteArtifactIdentityFields(&first) || !view.WriteArtifactIdentityFields(&second) {
		t.Fatal("diagnostic identity replay failed")
	}
	if fmt.Sprint(first.fields) != fmt.Sprint(second.fields) {
		t.Fatal("diagnostic identity replay was not deterministic")
	}
	if len(first.fields) < 2 || first.fields[0] != fmt.Sprintf("uint:%d", DiagnosticRowsLawVersion) || first.fields[1] != "uint:4" {
		t.Fatalf("diagnostic replay header = %v", first.fields[:min(2, len(first.fields))])
	}
	for _, required := range []string{"string:identity.lua", "string:pkg", "string:name", "string:missing", fmt.Sprintf("uint:%d", structure.DiagnosticObservationTypeConformance)} {
		found := false
		for _, field := range first.fields {
			if field == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("diagnostic replay omitted %q: %v", required, first.fields)
		}
	}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
