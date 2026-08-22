package programdiagnostic_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
)

func TestDiagnosticObservationVariantConstructorsRequireOwnedPayloadLaw(t *testing.T) {
	span := programsource.Span{File: "diagnostic.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	row, admitted := programdiagnostic.NewDiagnosticObservationValueReferenceUnresolved(
		identity.ContentID{1}, span, identity.ContentID{2}, identity.ContentID{3}, "missing",
	)
	if !admitted || !row.Available() {
		t.Fatal("valid unresolved-value observation unavailable")
	}
	_, admitted = programdiagnostic.NewDiagnosticObservationValueReferenceUnresolved(
		identity.ContentID{1}, span, identity.ContentID{2}, identity.ContentID{3}, "",
	)
	if admitted {
		t.Fatal("value-reference variant admitted an empty required name")
	}
	_, admitted = programdiagnostic.NewDiagnosticObservationTypeReferenceUnresolved(
		identity.ContentID{4}, span, 0, 0, identity.ContentID{5}, identity.ContentID{},
	)
	if admitted {
		t.Fatal("type-reference variant admitted an empty required path span")
	}
}

// TestCallArgumentObservationRequiresOwnerIssuedCalleeLaw states the semantic
// boundary for the direct-call site. The compiler owns the authored callee
// spelling while it still has the call relation, so a call row without that
// field is not a usable observation; assignment rows remain independent.
func TestCallArgumentObservationRequiresOwnerIssuedCalleeLaw(t *testing.T) {
	span := programsource.Span{File: "diagnostic.lua", StartLine: 2, StartCol: 18, EndLine: 2, EndCol: 26}
	row, admitted := programdiagnostic.NewDiagnosticObservationTypeConformance(
		identity.ContentID{10}, span, 1, 1,
		programdiagnostic.DiagnosticObservationSiteCallArgument,
		identity.ContentID{11}, identity.ContentID{12}, identity.ContentID{13}, identity.ContentID{14}, 0,
		"x", "takes_string",
	)
	if !admitted || !row.Available() || row.CalleeName() != "takes_string" {
		t.Fatal("call argument row did not retain its owner-issued callee")
	}
	_, admitted = programdiagnostic.NewDiagnosticObservationTypeConformance(
		identity.ContentID{10}, span, 1, 1,
		programdiagnostic.DiagnosticObservationSiteCallArgument,
		identity.ContentID{11}, identity.ContentID{12}, identity.ContentID{13}, identity.ContentID{14}, 0,
		"x",
	)
	if admitted {
		t.Fatal("call argument row without an owner-issued callee was admitted")
	}
	_, admitted = programdiagnostic.NewDiagnosticObservationTypeConformance(
		identity.ContentID{10}, span, 1, 1,
		programdiagnostic.DiagnosticObservationSiteCallArgument,
		identity.ContentID{11}, identity.ContentID{12}, identity.ContentID{13}, identity.ContentID{14}, 0,
		"x", "takes_string", "extra",
	)
	if admitted {
		t.Fatal("call argument row with an ambiguous callee payload was admitted")
	}
	assignment, admitted := programdiagnostic.NewDiagnosticObservationTypeConformance(
		identity.ContentID{20}, span, 1, 1,
		programdiagnostic.DiagnosticObservationSiteAssignment,
		identity.ContentID{21}, identity.ContentID{22}, identity.ContentID{23}, identity.ContentID{24}, 0,
		"x",
	)
	if !admitted || !assignment.Available() || assignment.CalleeName() != "" {
		t.Fatal("assignment row incorrectly depended on call-only callee payload")
	}
	if assignment.Site() != programdiagnostic.DiagnosticObservationSiteAssignment {
		t.Fatal("assignment row lost its conformance site")
	}
}
