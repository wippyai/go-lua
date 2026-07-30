package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

// TestEmptyTableAbsentFieldFunctionTypeRejected proves a never-written field of
// an empty table literal does not satisfy a declared function type. The field is
// nil at runtime, so trusting it as a function would let a non-function be
// called.
func TestEmptyTableAbsentFieldFunctionTypeRejected(t *testing.T) {
	result := Check(`local function f(): number
    local t = {}
    local g: fun(): number = t.run
    return g()
end return f`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "t.run") || !strings.Contains(diag.Message, "nil") {
		t.Fatalf("message = %q, want a nil mismatch on t.run", diag.Message)
	}
}

// TestEmptyTableAbsentFieldScalarTypeRejected proves a never-written field of an
// empty table literal reads as nil against a non-optional scalar target.
func TestEmptyTableAbsentFieldScalarTypeRejected(t *testing.T) {
	result := Check(`local function f(): number
    local t = {}
    local n: number = t.x
    return n
end return f`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "t.x") || !strings.Contains(diag.Message, "nil") {
		t.Fatalf("message = %q, want a nil mismatch on t.x", diag.Message)
	}
}

// TestEmptyTableAssignableToArray proves the empty literal still satisfies an
// array annotation.
func TestEmptyTableAssignableToArray(t *testing.T) {
	result := Check(`local function f(): number local a: {number} = {} return #a end return f`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// TestEmptyTableAssignableToMap proves the empty literal still satisfies a map
// annotation and a written key reads at its written type.
func TestEmptyTableAssignableToMap(t *testing.T) {
	result := Check(`local function f(): number local m: {[string]: number} = {} m.k = 1 return m.k end return f`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// TestEmptyTableWrittenFieldReadsWrittenType proves incremental table building is
// preserved: a written field reads back at its written type, not nil.
func TestEmptyTableWrittenFieldReadsWrittenType(t *testing.T) {
	result := Check(`local function f(): number local t = {} t.x = 1 local n: number = t.x return n end return f`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// TestNonEmptyClosedLiteralAbsentFieldRejected proves the closed-literal contrast
// is unchanged: a field absent from a non-empty literal reads as nil.
func TestNonEmptyClosedLiteralAbsentFieldRejected(t *testing.T) {
	result := Check(`local function f(): string local t = {a=1} local s: string = t.x return s end return f`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "t.x") || !strings.Contains(diag.Message, "nil") {
		t.Fatalf("message = %q, want a nil mismatch on t.x", diag.Message)
	}
}
