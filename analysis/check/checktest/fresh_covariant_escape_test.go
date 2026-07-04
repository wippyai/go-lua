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

// TestCastAliasWideViewAllowsWriteAndReportsAtNarrowRead pins mutable cast
// views: the cast result has the wider writable target type, while the original
// object is still exposed to that wider view and later reads from it are unsafe.
func TestCastAliasWideViewAllowsWriteAndReportsAtNarrowRead(t *testing.T) {
	result := Check(`local function f(): number
    local narrow: { x: number } = { x = 1 }
    local wide = narrow as { x: number | string }
    wide.x = "boom"
    local n: number = narrow.x
    return n
end return f`, WithStdlib())
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeAssignmentType && diag.Position.Line == 4 {
			t.Fatalf("unexpected assignment diagnostic on cast-view write: %#v", diag)
		}
	}
	requireAssignmentDiagnosticAtLineContaining(t, result, 5, "narrow.x")
}

// TestInterprocReturnMemberCovariantAliasReportsAtRead pins the return-member
// exposure route: a callee may store a parameter into a wider returned member.
// The caller then mutates through the returned view, so the original argument's
// field must read back at the wider type.
func TestInterprocReturnMemberCovariantAliasReportsAtRead(t *testing.T) {
	result := Check(`local function ibox(o: { x: number | string }): { ref: { x: number | string } } return { ref = o } end
local function f(): number
    local narrow: { x: number } = { x = 1 }
    local h = ibox(narrow)
    h.ref.x = "boom"
    local n: number = narrow.x
    return n
end return f`, WithStdlib())
	requireAssignmentDiagnosticAtLineContaining(t, result, 6, "narrow.x")
}

// TestInterprocParamStoreCovariantAliasReportsAtRead pins the param-to-param
// store route: the callee stores a source parameter into a wider member slot of
// another parameter, exposing the source object at that wider member type.
func TestInterprocParamStoreCovariantAliasReportsAtRead(t *testing.T) {
	result := Check(`local function ilink(dst: { ref: { x: number | string } }, o: { x: number | string }) dst.ref = o end
local function f(): number
    local narrow: { x: number } = { x = 1 }
    local holder: { ref: { x: number | string } } = { ref = { x = 0 } }
    ilink(holder, narrow)
    holder.ref.x = "boom"
    local n: number = narrow.x
    return n
end return f`, WithStdlib())
	requireAssignmentDiagnosticAtLineContaining(t, result, 7, "narrow.x")
}

// TestInterprocCapturedSinkCovariantAliasReportsAtRead pins the captured-sink
// route: the callee stores a parameter into a persistent wider sink the caller
// cannot locally rewrite into a same-frame alias.
func TestInterprocCapturedSinkCovariantAliasReportsAtRead(t *testing.T) {
	result := Check(`local isink: { ref: { x: number | string } } = { ref = { x = 0 } }
local function istash(o: { x: number | string }) isink.ref = o end
local function f(): number
    local narrow: { x: number } = { x = 1 }
    istash(narrow)
    isink.ref.x = "boom"
    local n: number = narrow.x
    return n
end return f`, WithStdlib())
	requireAssignmentDiagnosticAtLineContaining(t, result, 7, "narrow.x")
}

func requireAssignmentDiagnosticAtLineContaining(t *testing.T, result Result, line int, want string) {
	t.Helper()
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeAssignmentType && diag.Position.Line == line && strings.Contains(diag.Message, want) {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want assignment diagnostic on line %d containing %q", result.Diagnostics, line, want)
}
