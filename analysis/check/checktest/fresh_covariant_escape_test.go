package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

// TestFreshRecordEscapesToMutatingCalleeReportsAtRead proves that a fresh record
// literal passed to a callee that mutates it covariantly does not launder the
// write-through. Passing the literal as a call argument publishes it out of its
// construction frame, so it is no longer trusted for covariant widening: the
// wider callee parameter lets the callee store a string into the field the
// source declares as number, so reading narrow.x back as number is unsound and
// must report at the read, exactly as the parameter-sourced case does.
func TestFreshRecordEscapesToMutatingCalleeReportsAtRead(t *testing.T) {
	result := Check(`local function corrupt(w: { x: number | string }) w.x = "boom" end
local function f(): number
    local narrow: { x: number } = { x = 1 }
    corrupt(narrow)
    local n: number = narrow.x
    return n
end return f`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "narrow.x") || !strings.Contains(diag.Message, "number") {
		t.Fatalf("message = %q, want a write-through mismatch on the read of narrow.x", diag.Message)
	}
	if diag.Position.Line != 5 {
		t.Fatalf("line = %d, want the read of narrow.x on line 5", diag.Position.Line)
	}
}

// TestFreshRecordEscapesViaHelperReportsAtRead proves the same hole is closed
// when the fresh record is laundered through a helper return before the
// mutating call: returning the literal publishes it out of its construction
// frame just as a direct call argument does, so the covariant write-through is
// caught at the read.
func TestFreshRecordEscapesViaHelperReportsAtRead(t *testing.T) {
	result := Check(`local function build(): { x: number } return { x = 1 } end
local function corrupt(w: { x: number | string }) w.x = "boom" end
local function f(): number
    local narrow: { x: number } = build()
    corrupt(narrow)
    local n: number = narrow.x
    return n
end return f`, WithStdlib())
	requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
}

// TestParamSourcedCovariantArgStillReports proves the existing opaque
// (parameter-sourced) contrast is unchanged: a parameter record passed to a
// covariantly-mutating callee is already rejected at the read, and the
// fresh-literal fix makes fresh values behave identically.
func TestParamSourcedCovariantArgStillReports(t *testing.T) {
	result := Check(`local function corrupt(w: { x: number | string }) w.x = "boom" end
local function f(narrow: { x: number }): number
    corrupt(narrow)
    local n: number = narrow.x
    return n
end return f`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "narrow.x") {
		t.Fatalf("message = %q, want a write-through mismatch on the read of narrow.x", diag.Message)
	}
}

// TestFreshRecordLocalUseStaysClean proves a fresh record literal used purely
// locally, read at its declared type without escaping or covariant widening,
// does not spuriously report.
func TestFreshRecordLocalUseStaysClean(t *testing.T) {
	result := Check(`local function f(): number
    local narrow: { x: number } = { x = 1 }
    local n: number = narrow.x
    return n
end return f`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for a fresh record used purely locally", result.Diagnostics)
	}
}
