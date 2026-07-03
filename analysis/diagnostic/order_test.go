package diagnostic

import (
	"strings"
	"testing"
)

func TestSortOrdersDiagnosticsBySourcePosition(t *testing.T) {
	diags := []Diagnostic{
		{
			Position: Position{File: "main.lua", Line: 8, Column: 12},
			Code:     Code("type.assignment"),
			Message:  "later assignment",
			Severity: SeverityError,
		},
		{
			Position: Position{File: "main.lua", Line: 3, Column: 4},
			Code:     Code("type.call"),
			Message:  "earlier call",
			Severity: SeverityError,
		},
		{
			Code:     Code("parse"),
			Message:  "no position",
			Severity: SeverityError,
		},
		{
			Position: Position{File: "helper.lua", Line: 1, Column: 1},
			Code:     Code("lint"),
			Message:  "other file",
			Severity: SeverityWarning,
		},
	}

	Sort(diags)

	got := []string{diags[0].Message, diags[1].Message, diags[2].Message, diags[3].Message}
	want := []string{"other file", "earlier call", "later assignment", "no position"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted[%d] = %q, want %q; full order = %#v", i, got[i], want[i], got)
		}
	}
}

func TestSortUsesDeterministicTieBreakers(t *testing.T) {
	diags := []Diagnostic{
		{
			Position: Position{File: "main.lua", Line: 1, Column: 1},
			Code:     Code("b"),
			Message:  "same span",
			Severity: SeverityWarning,
		},
		{
			Position: Position{File: "main.lua", Line: 1, Column: 1},
			Code:     Code("a"),
			Message:  "same span",
			Severity: SeverityError,
		},
	}

	Sort(diags)

	if diags[0].Severity != SeverityError || diags[0].Code != Code("a") {
		t.Fatalf("same-span order = %#v, want error/code tie-breaker first", diags)
	}
}

func TestSortUsesDeterministicTieBreakersForPositionlessDiagnostics(t *testing.T) {
	diags := []Diagnostic{
		{Code: Code("lint.unused"), Message: "later lint", Severity: SeverityHint},
		{Code: Code("parse.syntax"), Message: "parse failed", Severity: SeverityError},
		{Code: Code("lint.unused"), Message: "earlier lint", Severity: SeverityHint},
		{Code: Code("type.assignment"), Message: "type failed", Severity: SeverityError},
	}

	Sort(diags)

	got := []string{diags[0].Message, diags[1].Message, diags[2].Message, diags[3].Message}
	want := []string{"parse failed", "type failed", "earlier lint", "later lint"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("positionless sorted[%d] = %q, want %q; full order = %#v", i, got[i], want[i], got)
		}
	}
}

func TestDeduplicateRemovesExactDiagnosticsOnly(t *testing.T) {
	span := Span{StartLine: 4, StartCol: 2, EndLine: 4, EndCol: 5}
	base := New(DiagnosticSpec{
		File:     "main.lua",
		Span:     span,
		Code:     Code("type.assignment"),
		Message:  "cannot assign",
		Severity: SeverityError,
		Explanation: NewExplanation(Evidence{
			Kind:    EvidenceAbstractFact,
			Trust:   TrustProven,
			Span:    span,
			Message: "value is number",
		}),
		Help:   "use a string",
		Labels: []Label{{Span: span, Message: "assigned value", Placement: LabelPlacementBelow}},
	})
	distinctEvidence := base
	distinctEvidence.Explanation = NewExplanation(Evidence{
		Kind:    EvidenceAbstractFact,
		Trust:   TrustProven,
		Span:    span,
		Message: "value is integer",
	})

	got := Deduplicate([]Diagnostic{base, base, distinctEvidence})
	if len(got) != 2 {
		t.Fatalf("deduplicated diagnostics = %#v, want exact duplicate removed but distinct evidence kept", got)
	}
	if got[0].Explanation.String() == got[1].Explanation.String() {
		t.Fatalf("distinct evidence diagnostics collapsed: %#v", got)
	}
}

func TestCoalesceSamePrimaryRemovesRepeatedUserVisibleDiagnostics(t *testing.T) {
	span := Span{StartLine: 4, StartCol: 2, EndLine: 4, EndCol: 5}
	base := New(DiagnosticSpec{
		File:     "main.lua",
		Span:     span,
		Code:     Code("type.call.direct.argument_type"),
		Message:  "argument 1 comes from any/unknown; no proof shows it is string",
		Severity: SeverityError,
		Explanation: NewExplanation(Evidence{
			Kind:    EvidenceUserAssertion,
			Trust:   TrustClaimed,
			Span:    span,
			Message: "argument 1 is claimed as string",
		}),
		Help: "validate first",
	})
	alternateEvidence := base
	alternateEvidence.Explanation = NewExplanation(Evidence{
		Kind:    EvidenceMissingProof,
		Trust:   TrustUnknown,
		Span:    span,
		Message: "no proof on this path shows argument 1 is string",
	})
	differentMessage := base
	differentMessage.Message = "argument 1 is number, not string"

	got := CoalesceSamePrimary([]Diagnostic{base, alternateEvidence, differentMessage})
	if len(got) != 2 {
		t.Fatalf("coalesced diagnostics = %#v, want repeated primary collapsed and distinct message kept", got)
	}
	if got[0].Message != base.Message || got[1].Message != differentMessage.Message {
		t.Fatalf("coalesced order/messages = %#v", got)
	}
}

func TestCoalesceSamePrimaryPrefersMoreSpecificEvidence(t *testing.T) {
	span := Span{StartLine: 4, StartCol: 2, EndLine: 4, EndCol: 5}
	broad := New(DiagnosticSpec{
		File:     "main.lua",
		Span:     span,
		Code:     Code("type.operator.concat_operand"),
		Message:  "right operand of `..` may be nil",
		Severity: SeverityWarning,
		Explanation: NewExplanation(Evidence{
			Kind:    EvidenceAbstractFact,
			Trust:   TrustProven,
			Reason:  EvidenceReasonUnionType,
			Span:    span,
			Message: "right operand `maybe` can be string or nil here",
		}),
	})
	exact := broad
	exact.Explanation = NewExplanation(Evidence{
		Kind:    EvidenceAbstractFact,
		Trust:   TrustProven,
		Reason:  EvidenceReasonExactType,
		Span:    span,
		Message: "right operand `maybe` has type nil",
	})

	got := CoalesceSamePrimary([]Diagnostic{broad, exact})
	if len(got) != 1 {
		t.Fatalf("coalesced diagnostics = %#v, want one", got)
	}
	if evidence := got[0].Explanation.String(); !strings.Contains(evidence, "has type nil") {
		t.Fatalf("coalesced evidence = %q, want exact evidence", evidence)
	}
}
