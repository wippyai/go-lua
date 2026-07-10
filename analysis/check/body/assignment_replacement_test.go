package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestInferredReplacementAcceptedAllowsNumberForInferredInteger(t *testing.T) {
	result := &Result{}
	target := OrdinaryAssignmentTargetType{Declared: false}

	if !result.InferredReplacementAccepted(0, target, typ.Integer, typ.Number) {
		t.Fatal("inferred integer target should accept number replacement")
	}
}

func TestInferredReplacementAcceptedRejectsDeclaredTarget(t *testing.T) {
	result := &Result{}
	target := OrdinaryAssignmentTargetType{Declared: true}

	if result.InferredReplacementAccepted(0, target, typ.Integer, typ.Number) {
		t.Fatal("declared integer target should not accept number replacement")
	}
}

func TestInferredReplacementAcceptedAllowsBroadTableForInferredRecord(t *testing.T) {
	result := &Result{}
	target := OrdinaryAssignmentTargetType{Declared: false}
	expected := table.NewRecord().Field("id", typ.String).Build()

	if !result.InferredReplacementAccepted(0, target, expected, table.BuiltinTopMarker()) {
		t.Fatal("inferred record target should accept broad table replacement")
	}
}

func TestInferredReplacementAcceptedRejectsOptionalRecordReplacement(t *testing.T) {
	result := &Result{}
	target := OrdinaryAssignmentTargetType{Declared: false}
	expected := table.NewRecord().Field("id", typ.String).Build()
	actual := typ.MaterializeOptional(table.NewRecord().Field("id", typ.String).Build())

	if result.InferredReplacementAccepted(0, target, expected, actual) {
		t.Fatal("optional record replacement should not be accepted")
	}
}
