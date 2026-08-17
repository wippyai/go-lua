package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func TestDiagnosticObservationRowsRejectMixedPayloadFamilies(t *testing.T) {
	span := programsource.Span{File: "diagnostic.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	row := DiagnosticObservationRow{id: identity.ContentID{1}, kind: DiagnosticObservationValueReferenceUnresolved, location: span, value: diagnosticUnresolvedValueReferenceRow{read: identity.ContentID{2}, cell: identity.ContentID{3}, name: "missing"}}
	if !row.Available() {
		t.Fatal("valid unresolved-value observation unavailable")
	}
	row.branch = diagnosticBranchConditionRow{decision: identity.ContentID{4}, value: identity.ContentID{5}, points: []identity.ContentID{{6}}}
	if row.Available() {
		t.Fatal("observation admitted a foreign payload family")
	}
}
