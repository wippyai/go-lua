package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

// A call that supplies fewer values than there are destructuring targets leaves
// the surplus targets nil at runtime. The checker must type those surplus
// targets nil, not their declared type, so a later use against a non-nil type is
// rejected. This mirrors the literal under-supply path (`local a, b = 1`), which
// already nil-pads the surplus slot.
func TestCheckCallUnderSupplySurplusTargetIsNil(t *testing.T) {
	src := `
local function one(): number return 1 end
local a: number, b: number = one()
return a
`
	requireDiagnostic(t, Check(src, WithStdlib()), diagnosticExpectation{
		DiagnosticCount: 1,
		Code:            diagnostics.CodeAssignmentType,
		Line:            3,
		MessageContains: []string{"cannot assign b", "nil", "number"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"b has type nil"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"b is declared as number"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"b receives result 2 from `one(...)`", "no value was produced"},
			},
		},
		LabelContains: []string{"declared type"},
		HelpContains:  []string{"Provide a value for `b`", "change the target type to accept nil"},
		Sources: diagnostic.SourceMap{
			"test.lua": src,
		},
		RenderOrderedContains: []string{
			"cannot assign b because it is nil, not number",
			"declared type",
			"local a: number, b: number = one()",
			"b has type nil",
			"b is declared as number",
			"b receives result 2 from `one(...)`",
			"help: Provide a value for `b`",
		},
		RenderNotContains: []string{"assigned value"},
	})
}

// A third target supplied by a two-value call is nil for the same reason.
func TestCheckCallUnderSupplyThirdTargetIsNil(t *testing.T) {
	src := `
local function two(): number, number return 1, 2 end
local a: number, b: number, c: number = two()
return a
`
	requireDiagnostic(t, Check(src, WithStdlib()), diagnosticExpectation{
		DiagnosticCount: 1,
		Code:            diagnostics.CodeAssignmentType,
		Line:            3,
		MessageContains: []string{"cannot assign c", "nil", "number"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"c has type nil"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"c is declared as number"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"c receives result 3 from `two(...)`", "no value was produced"},
			},
		},
		LabelContains: []string{"declared type"},
		HelpContains:  []string{"Provide a value for `c`", "change the target type to accept nil"},
	})
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
