package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

// A call that supplies fewer values than there are destructuring targets leaves
// the surplus targets nil at runtime. The checker must type those surplus
// targets nil, not their declared type, so a later use against a non-nil type is
// rejected. This mirrors the literal under-supply path (`local a, b = 1`), which
// already nil-pads the surplus slot.
func TestCheckCallUnderSupplySurplusTargetIsNil(t *testing.T) {
	result := Check(`
local function one(): number return 1 end
local a: number, b: number = one()
local c: number = b
return c
`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "b") || !strings.Contains(diag.Message, "nil") || !strings.Contains(diag.Message, "number") {
		t.Fatalf("message = %q, want nil-vs-number assignment diagnostic for `b`", diag.Message)
	}
}

// A third target supplied by a two-value call is nil for the same reason.
func TestCheckCallUnderSupplyThirdTargetIsNil(t *testing.T) {
	result := Check(`
local function two(): number, number return 1, 2 end
local a: number, b: number, c: number = two()
local d: number = c
return d
`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "c") || !strings.Contains(diag.Message, "nil") {
		t.Fatalf("message = %q, want nil assignment diagnostic for surplus target `c`", diag.Message)
	}
}

// A callee whose return arity is statically unknown (a function-typed parameter
// resolved only at the boundary) must not be nil-padded: padding there would be
// unsound the other way (over-strict on values the callee may actually return).
func TestCheckCallUnknownArityNotPaddedToNil(t *testing.T) {
	result := Check(`
local function f(g: fun(): number): number
    local a: number, b: number = g()
    local c: number = b
    return c
end
return f
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unknown-arity callee under-supply", result.Diagnostics)
	}
}

// A correctly supplied multi-return destructuring stays clean: every target is
// within the callee's declared return arity.
func TestCheckCallCorrectMultiReturnStaysClean(t *testing.T) {
	result := Check(`
local function two(): number, number return 1, 2 end
local a: number, b: number = two()
local c: number = b
return c
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for in-arity multi-return destructuring", result.Diagnostics)
	}
}
