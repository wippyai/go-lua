package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

// TestCovariantArrayAliasOpaqueWriteThroughReportsAtRead proves that a covariant
// alias of an opaque (parameter) array is sound: the alias widens the source's
// element type, so reading the source element back into the narrower declared
// type surfaces the unsound write-through.
func TestCovariantArrayAliasOpaqueWriteThroughReportsAtRead(t *testing.T) {
	result := Check(`local function f(a: {string}): string
    local b: {string | number} = a
    b[1] = 42
    local s: string = a[1]
    return s
end return f`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "number") || !strings.Contains(diag.Message, "string") {
		t.Fatalf("message = %q, want element-type mismatch on the widened read", diag.Message)
	}
	if diag.Position.Line != 4 {
		t.Fatalf("line = %d, want the read of a[1] on line 4", diag.Position.Line)
	}
}

// TestCovariantArrayAliasPureReadStaysSound proves a pure covariant read through
// an alias of an opaque array does not spuriously report: no write occurs, so
// the source element type still reads at its declared type.
func TestCovariantArrayAliasPureReadStaysSound(t *testing.T) {
	result := Check(`local function f(a: {string})
    local b: {string | number} = a
    local x = b[1]
    return x
end return f`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for a pure covariant read of a parameter array", result.Diagnostics)
	}
}

// TestCovariantArrayAliasReadOnlyPassThroughStaysSound proves passing an opaque
// {string} array where a wider {string | number} array is read does not report:
// covariant reads of parameter arrays remain accepted.
func TestCovariantArrayAliasReadOnlyPassThroughStaysSound(t *testing.T) {
	result := Check(`local function need(xs: {string | number}): number return 1 end
local function f(a: {string}): number
    return need(a)
end return f`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for a read-only covariant pass-through", result.Diagnostics)
	}
}

// TestEqualElementArrayAliasStaysSound proves an equal-element alias is not
// widened and does not report: no covariant widening occurs.
func TestEqualElementArrayAliasStaysSound(t *testing.T) {
	result := Check(`local function f(a: {string})
    local b: {string} = a
    local x = b[1]
    return x
end return f`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for an equal-element array alias", result.Diagnostics)
	}
}
