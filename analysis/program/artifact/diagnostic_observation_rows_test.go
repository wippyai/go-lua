package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestDiagnosticObservationVariantConstructorsRequireOwnedPayload(t *testing.T) {
	span := programsource.Span{File: "diagnostic.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	row, admitted := programschema.NewDiagnosticObservationValueReferenceUnresolved(
		identity.ContentID{1}, span, identity.ContentID{2}, identity.ContentID{3}, "missing",
	)
	if !admitted || !row.Available() {
		t.Fatal("valid unresolved-value observation unavailable")
	}
	_, admitted = programschema.NewDiagnosticObservationValueReferenceUnresolved(
		identity.ContentID{1}, span, identity.ContentID{2}, identity.ContentID{3}, "",
	)
	if admitted {
		t.Fatal("value-reference variant admitted an empty required name")
	}
	_, admitted = programschema.NewDiagnosticObservationTypeReferenceUnresolved(
		identity.ContentID{4}, span, 0, 0, identity.ContentID{5}, identity.ContentID{},
	)
	if admitted {
		t.Fatal("type-reference variant admitted an empty required path span")
	}
}
