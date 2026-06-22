package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

// A cast adopts its target type for inference, but it must not prove a separate
// obligation that the uncast value could not. These probes guard that an any or
// disjoint value cannot launder past a parameter, assignment, or return contract
// through a cast, while a cast used purely for local inference stays clean.

func TestCastDoesNotLaunderAnyIntoParameter(t *testing.T) {
	src := strings.TrimLeft(`
local function need(x: {name: string}): number return 1 end
local function f(y: any): number return need(y as {name: string}) end return f
`, "\n")
	result := Check(src)
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            2,
		Column:          46,
		MessageContains: []string{
			"argument 1 (y)",
			"any",
			"not {name: string}",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"argument 1 (y)", "has type any"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"need parameter 1 expects {name: string}"},
			},
			{
				Kind:            diagnostic.EvidencePrecisionBoundary,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"argument 1 (y) comes from any/unknown"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no proof on this path", "y", "parameter type"},
			},
		},
		LabelContains: []string{"argument value"},
		HelpContains:  []string{"Validate or narrow `y`", "any/unknown values do not prove parameter contracts"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.call.direct.argument_type]: argument 1 (y) is any, not {name: string}`,
			`2 | local function f(y: any): number return need(y as {name: string}) end return f`,
			`  |                                              ↑ argument value`,
			`1. proven: argument 1 (y) has type any`,
			`2. claimed: need parameter 1 expects {name: string}`,
			`1 | local function need(x: {name: string}): number return 1 end`,
			`3. unvalidated value: argument 1 (y) comes from any/unknown`,
			`4. missing proof: no proof on this path shows y satisfies the parameter type`,
			"help: Validate or narrow `y` before passing it; any/unknown values do not prove parameter contracts.",
		},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.argument_type]: argument 1 (y) is any, not {name: string}
 --> test.lua:2:46
  |
2 | local function f(y: any): number return need(y as {name: string}) end return f
  |                                              ↑ argument value

because:
  1. proven: argument 1 (y) has type any
  2. claimed: need parameter 1 expects {name: string}
 --> test.lua:1:24
  |
1 | local function need(x: {name: string}): number return 1 end
  |                        ^
  3. unvalidated value: argument 1 (y) comes from any/unknown
  4. missing proof: no proof on this path shows y satisfies the parameter type

help: Validate or narrow ` + "`y`" + ` before passing it; any/unknown values do not prove parameter contracts.`
	assertRenderedEqual(t, rendered, want)
}

func TestCastDoesNotLaunderDisjointValueIntoParameter(t *testing.T) {
	result := Check(`
local function need(x: {name: string}): number return 1 end
local function f(y: number): number return need(y as {name: string}) end return f
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	if got := diag.Message; got != "argument 1 (y) is number, not {name: string}" {
		t.Fatalf("message = %q, want number-not-record argument mismatch", got)
	}
}

func TestCastDoesNotLaunderAnyIntoAnnotatedAssignment(t *testing.T) {
	src := strings.TrimLeft(`
local function f(y: any): number
	local x: {name: string} = y as {name: string}
	return 1
end
return f
`, "\n")
	result := Check(src)
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            2,
		Column:          28,
		MessageContains: []string{
			"cannot assign y",
			"any",
			"not {name: string}",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"y has type any"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"x is declared as {name: string}"},
			},
			{
				Kind:            diagnostic.EvidencePrecisionBoundary,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"y comes from any/unknown"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no proof on this path", "y", "declared type"},
			},
		},
		LabelContains: []string{"declared type", "assigned value"},
		HelpContains:  []string{"Use a value compatible", "change the target type", "`y` is valid"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.assignment]: cannot assign y because it is any, not {name: string}`,
			`  |              ↓ declared type`,
			`2 |     local x: {name: string} = y as {name: string}`,
			`  |                               ↑ assigned value`,
			`1. proven: y has type any`,
			`2. claimed: x is declared as {name: string}`,
			`3. unvalidated value: y comes from any/unknown`,
			`4. missing proof: no proof on this path shows y satisfies the declared type`,
			"help: Use a value compatible with the expected type, or change the target type if `y` is valid.",
		},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.assignment]: cannot assign y because it is any, not {name: string}
 --> test.lua:2:28
  |
  |              ↓ declared type
2 |     local x: {name: string} = y as {name: string}
  |                               ↑ assigned value

because:
  1. proven: y has type any
  2. claimed: x is declared as {name: string}
  3. unvalidated value: y comes from any/unknown
  4. missing proof: no proof on this path shows y satisfies the declared type

help: Use a value compatible with the expected type, or change the target type if ` + "`y`" + ` is valid.`
	assertRenderedEqual(t, rendered, want)
}

func TestCastDoesNotLaunderAnyIntoReturnContract(t *testing.T) {
	src := strings.TrimLeft(`
local function f(y: any): {name: string} return y as {name: string} end return f
`, "\n")
	result := Check(src)
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            1,
		Column:          49,
		MessageContains: []string{
			"returned value 1 (y)",
			"any",
			"not {name: string}",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"returned value 1 (y)", "has type any"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"returned value 1 must satisfy declared return type {name: string}"},
			},
			{
				Kind:            diagnostic.EvidencePrecisionBoundary,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"returned value 1 (y) comes from any/unknown"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no proof on this path", "returned value 1 (y)", "declared return type"},
			},
		},
		LabelContains: []string{"declared return type", "returned value"},
		HelpContains:  []string{"Return a value compatible", "change the return annotation", "returned value is valid"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.return.contract]: returned value 1 (y) is any, not {name: string}`,
			`  |                           ↓ declared return type`,
			`1 | local function f(y: any): {name: string} return y as {name: string} end return f`,
			`  |                                                 ↑ returned value`,
			`1. proven: returned value 1 (y) has type any`,
			`2. claimed: returned value 1 must satisfy declared return type {name: string}`,
			`3. unvalidated value: returned value 1 (y) comes from any/unknown`,
			`4. missing proof: no proof on this path shows returned value 1 (y) satisfies the declared return type`,
			`help: Return a value compatible with the declared return type, or change the return annotation if the returned value is valid.`,
		},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.return.contract]: returned value 1 (y) is any, not {name: string}
 --> test.lua:1:49
  |
  |                           ↓ declared return type
1 | local function f(y: any): {name: string} return y as {name: string} end return f
  |                                                 ↑ returned value

because:
  1. proven: returned value 1 (y) has type any
  2. claimed: returned value 1 must satisfy declared return type {name: string}
  3. unvalidated value: returned value 1 (y) comes from any/unknown
  4. missing proof: no proof on this path shows returned value 1 (y) satisfies the declared return type

help: Return a value compatible with the declared return type, or change the return annotation if the returned value is valid.`
	assertRenderedEqual(t, rendered, want)
}

func TestCastDoesNotLaunderAnyIntoFieldAssignment(t *testing.T) {
	src := strings.TrimLeft(`
local function f(y: any, o: {name: string}): number
	o.name = y as string
	return 1
end
return f
`, "\n")
	result := Check(src)
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            2,
		Column:          11,
		MessageContains: []string{
			"cannot assign y",
			"any",
			"not string",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"y has type any"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"assignment target o.name requires string"},
			},
			{
				Kind:            diagnostic.EvidencePrecisionBoundary,
				Trust:           diagnostic.TrustUnknown,
				Reason:          diagnostic.EvidenceReasonExplicitBoundaryValidation,
				MessageContains: []string{"y comes from any/unknown"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				Reason:          diagnostic.EvidenceReasonBoundaryValidationMissing,
				MessageContains: []string{"no proof on this path", "y", "declared type"},
			},
		},
		LabelContains: []string{"assignment target", "assigned value"},
		HelpContains:  []string{"Use a value compatible", "change the target type", "`y` is valid"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.assignment]: cannot assign y because it is any, not string`,
			`  |     ↓ assignment target`,
			`2 |     o.name = y as string`,
			`  |              ↑ assigned value`,
			`1. proven: y has type any`,
			`2. proven: assignment target o.name requires string`,
			`3. unvalidated value: y comes from any/unknown`,
			`4. missing proof: no proof on this path shows y satisfies the declared type`,
			"help: Use a value compatible with the expected type, or change the target type if `y` is valid.",
		},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.assignment]: cannot assign y because it is any, not string
 --> test.lua:2:11
  |
  |     ↓ assignment target
2 |     o.name = y as string
  |              ↑ assigned value

because:
  1. proven: y has type any
  2. proven: assignment target o.name requires string
  3. unvalidated value: y comes from any/unknown
  4. missing proof: no proof on this path shows y satisfies the declared type

help: Use a value compatible with the expected type, or change the target type if ` + "`y`" + ` is valid.`
	assertRenderedEqual(t, rendered, want)
}

func TestCastAdoptsTypeForLocalInference(t *testing.T) {
	result := Check(`
local function f(y: any): string local r = y as {name: string} return r.name end return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for cast adopted as local inference type", result.Diagnostics)
	}
}

func TestGuardedOptionalCastSatisfiesParameter(t *testing.T) {
	// The wrapped value is already proven (a guarded string? is string), so the
	// redundant cast does not turn a sound argument into an error.
	result := Check(`
local function need(s: string): number return 1 end
local function f(t: {cond: string?}): number
	if t.cond then
		return need(t.cond :: string)
	end
	return 0
end
return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for guarded-optional cast argument", result.Diagnostics)
	}
}
