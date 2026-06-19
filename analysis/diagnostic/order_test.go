package diagnostic

import "testing"

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
