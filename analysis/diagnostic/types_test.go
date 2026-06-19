package diagnostic

import (
	"testing"
)

func TestSeverityString(t *testing.T) {
	tests := []struct {
		s    Severity
		want string
	}{
		{SeverityError, "error"},
		{SeverityWarning, "warning"},
		{SeverityHint, "hint"},
		{Severity(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestSeverityOrdering(t *testing.T) {
	if SeverityError >= SeverityWarning {
		t.Error("SeverityError should be less than SeverityWarning")
	}
	if SeverityWarning >= SeverityHint {
		t.Error("SeverityWarning should be less than SeverityHint")
	}
}

func TestPosition_String(t *testing.T) {
	tests := []struct {
		pos  Position
		want string
	}{
		{Position{File: "test.lua", Line: 10, Column: 5}, "test.lua:10:5"},
		{Position{Line: 10, Column: 5}, "10:5"},
		{Position{File: "", Line: 1, Column: 1}, "1:1"},
	}

	for _, tt := range tests {
		if got := tt.pos.String(); got != tt.want {
			t.Errorf("Position.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestPosition_Valid(t *testing.T) {
	tests := []struct {
		pos  Position
		want bool
	}{
		{Position{Line: 1, Column: 1}, true},
		{Position{File: "test.lua", Line: 10, Column: 5}, true},
		{Position{Line: 0, Column: 1}, false},
		{Position{Line: 1, Column: 0}, false},
		{Position{}, false},
	}

	for _, tt := range tests {
		if got := tt.pos.Valid(); got != tt.want {
			t.Errorf("Position.Valid() = %v, want %v for %+v", got, tt.want, tt.pos)
		}
	}
}

func TestSpan_Valid(t *testing.T) {
	tests := []struct {
		span Span
		want bool
	}{
		{Span{StartLine: 1, StartCol: 1}, true},
		{Span{StartLine: 0, StartCol: 1}, false},
		{Span{StartLine: 1, StartCol: 0}, false},
		{Span{}, false},
	}

	for _, tt := range tests {
		if got := tt.span.Valid(); got != tt.want {
			t.Errorf("Span.Valid() = %v, want %v for %+v", got, tt.want, tt.span)
		}
	}
}

func TestSpan_SingleLine(t *testing.T) {
	tests := []struct {
		span Span
		want bool
	}{
		{Span{StartLine: 1, EndLine: 1}, true},
		{Span{StartLine: 1, EndLine: 0}, true},
		{Span{StartLine: 1, EndLine: 2}, false},
	}

	for _, tt := range tests {
		if got := tt.span.SingleLine(); got != tt.want {
			t.Errorf("Span.SingleLine() = %v, want %v for %+v", got, tt.want, tt.span)
		}
	}
}

func TestPositionFromSpan(t *testing.T) {
	span := Span{StartLine: 3, StartCol: 9, EndLine: 3, EndCol: 14}
	got := PositionFromSpan(span)
	want := Position{Line: 3, Column: 9, EndLine: 3, EndColumn: 14}
	if got != want {
		t.Fatalf("PositionFromSpan = %#v, want %#v", got, want)
	}
}

func TestPositionFromSpanInFile(t *testing.T) {
	span := Span{StartLine: 3, StartCol: 9, EndLine: 3, EndCol: 14}
	got := PositionFromSpanInFile("main.lua", span)
	want := Position{File: "main.lua", Line: 3, Column: 9, EndLine: 3, EndColumn: 14}
	if got != want {
		t.Fatalf("PositionFromSpanInFile = %#v, want %#v", got, want)
	}
}

func TestNewDiagnosticDerivesPositionFromSpan(t *testing.T) {
	span := Span{StartLine: 4, StartCol: 12, EndLine: 4, EndCol: 18}
	labels := []Label{{Span: span, Message: "primary"}}
	got := New(DiagnosticSpec{
		File:     "main.lua",
		Span:     span,
		Code:     Code("type.example"),
		Message:  "example mismatch",
		Severity: SeverityWarning,
		Help:     "fix it",
		Labels:   labels,
	})
	if got.Position != (Position{File: "main.lua", Line: 4, Column: 12, EndLine: 4, EndColumn: 18}) {
		t.Fatalf("position = %#v, want position derived from span", got.Position)
	}
	if got.Span != span || got.Code != Code("type.example") || got.Message != "example mismatch" || got.Severity != SeverityWarning || got.Help != "fix it" {
		t.Fatalf("diagnostic = %#v, want spec fields preserved", got)
	}
	labels[0].Message = "mutated"
	if got.Labels[0].Message != "primary" {
		t.Fatalf("labels alias caller slice: %#v", got.Labels)
	}
}

func TestCodeString(t *testing.T) {
	tests := []struct {
		code Code
		want string
	}{
		{Code("type.mismatch"), "type.mismatch"},
		{Code(""), "diagnostic"},
	}

	for _, tt := range tests {
		if got := tt.code.String(); got != tt.want {
			t.Errorf("Code(%q).String() = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestDiagnostic_StringMethod(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "test.lua", Line: 10, Column: 5},
		Message:  "test error",
	}
	got := d.String()
	want := "test.lua:10:5: test error"
	if got != want {
		t.Errorf("Diagnostic.String() = %q, want %q", got, want)
	}
}

func TestDiagnostic_StringMethodWithoutFile(t *testing.T) {
	d := Diagnostic{
		Position: Position{Line: 10, Column: 5},
		Message:  "test error",
	}
	got := d.String()
	want := "10:5: test error"
	if got != want {
		t.Errorf("Diagnostic.String() = %q, want %q", got, want)
	}
}

func TestDiagnostic_StringMethodWithoutPosition(t *testing.T) {
	d := Diagnostic{Message: "test error"}
	got := d.String()
	want := "test error"
	if got != want {
		t.Errorf("Diagnostic.String() = %q, want %q", got, want)
	}
}

func TestDiagnostic_ErrorMethod(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 1},
		Message:  "syntax error",
	}
	got := d.Error()
	want := "main.lua:1:1: syntax error"
	if got != want {
		t.Errorf("Diagnostic.Error() = %q, want %q", got, want)
	}
}

func TestLabel(t *testing.T) {
	label := Label{
		File:    "main.lua",
		Span:    Span{StartLine: 1, StartCol: 0, EndLine: 1, EndCol: 10},
		Message: "here",
	}
	if label.File != "main.lua" {
		t.Errorf("Label.File = %q, want %q", label.File, "main.lua")
	}
	if label.Message != "here" {
		t.Errorf("Label.Message = %q, want %q", label.Message, "here")
	}
}
