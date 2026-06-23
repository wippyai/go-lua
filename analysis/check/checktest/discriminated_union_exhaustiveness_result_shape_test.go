package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDiscriminatedUnionExhaustivenessHandlesResultShape(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    if result.ok then
        return result.value
    end
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"result.ok == false"},
		EvidenceOrdered: []string{
			"branch chain checks discriminant `result.ok`",
			"handled cases: `result.ok == true`",
			"missing cases: `result.ok == false`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsUnguardedResultValueRead(t *testing.T) {
	src := strings.TrimLeft(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    return result.value
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            4,
		Column:          12,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"result.value",
			"result.ok == true",
		},
		EvidenceOrdered: []string{
			"`result` is a union discriminated by `result.ok`",
			"`result.value` exists only for `result.ok == true`",
			"no stable guard proves `result.ok == true` before this read",
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; ` + "`result.value`" + ` requires ` + "`result.ok == true`" + `
 --> test.lua:4:12
  |
4 |     return result.value
  |            ↑ case-specific field read

because:
  1. proven: ` + "`result`" + ` is a union discriminated by ` + "`result.ok`" + `
  2. proven: ` + "`result.value`" + ` exists only for ` + "`result.ok == true`" + `
  3. missing proof: no stable guard proves ` + "`result.ok == true`" + ` before this read

help: Check the union case before reading this field, or return from the opposite case before continuing.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessReportsDeepResultShapeEnvelope(t *testing.T) {
	src := strings.TrimLeft(`
type Ok = { envelope: { payload: { ok: true } }, value: string }
type Err = { envelope: { payload: { ok: false } }, error: string }
type Result = Ok | Err

local function use(result: Result): string
    return result.value
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            6,
		Column:          12,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"result.value",
			"result.envelope.payload.ok == true",
		},
		EvidenceOrdered: []string{
			"`result` is a union discriminated by `result.envelope.payload.ok`",
			"`result.value` exists only for `result.envelope.payload.ok == true`",
			"no stable guard proves `result.envelope.payload.ok == true` before this read",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result` is a union discriminated by `result.envelope.payload.ok`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result.value` exists only for `result.envelope.payload.ok == true`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no stable guard proves `result.envelope.payload.ok == true` before this read"},
			},
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; ` + "`result.value`" + ` requires ` + "`result.envelope.payload.ok == true`" + `
 --> test.lua:6:12
  |
6 |     return result.value
  |            ↑ case-specific field read

because:
  1. proven: ` + "`result`" + ` is a union discriminated by ` + "`result.envelope.payload.ok`" + `
  2. proven: ` + "`result.value`" + ` exists only for ` + "`result.envelope.payload.ok == true`" + `
  3. missing proof: no stable guard proves ` + "`result.envelope.payload.ok == true`" + ` before this read

help: Check the union case before reading this field, or return from the opposite case before continuing.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessAcceptsGuardedDeepResultShapeEnvelope(t *testing.T) {
	result := Check(`
type Ok = { envelope: { payload: { ok: true } }, value: string }
type Err = { envelope: { payload: { ok: false } }, error: string }
type Result = Ok | Err

local function use(result: Result): string
    if result.envelope.payload.ok then
        return result.value
    else
        return ""
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no result-shape exhaustiveness warning after deep discriminant guard", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessReportsUnguardedResultErrorRead(t *testing.T) {
	src := strings.TrimLeft(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    return result.error
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            4,
		Column:          12,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"result.error",
			"result.ok == false",
		},
		EvidenceMin: 3,
		EvidenceOrdered: []string{
			"`result` is a union discriminated by `result.ok`",
			"`result.error` exists only for `result.ok == false`",
			"no stable guard proves `result.ok == false` before this read",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result` is a union discriminated by `result.ok`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result.error` exists only for `result.ok == false`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no stable guard proves `result.ok == false` before this read"},
			},
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; `result.error` requires `result.ok == false`",
			"test.lua:4:12",
			"4 |     return result.error",
			"↑ case-specific field read",
			"because:",
			"proven: `result` is a union discriminated by `result.ok`",
			"proven: `result.error` exists only for `result.ok == false`",
			"missing proof: no stable guard proves `result.ok == false` before this read",
			"help: Check the union case before reading this field",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsGuardedResultReads(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    if result.ok then
        return result.value
    else
        return result.error
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no result-shape exhaustiveness warning for guarded reads", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsResultReadThroughEquivalentAliasGuard(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    local alias = result
    if alias.ok then
        return result.value
    else
        return ""
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want alias guard to prove equivalent result read", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsAliasResultReadThroughOriginalGuard(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    local alias = result
    if result.ok then
        return alias.value
    else
        return ""
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want original guard to prove equivalent alias read", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessDoesNotUseReassignedAliasGuardForOriginalResult(t *testing.T) {
	src := strings.TrimLeft(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>, replacement: Result<string>): string
    local alias = result
    alias = replacement
    if alias.ok then
        return result.value
    else
        return ""
    end
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            7,
		Column:          16,
		MessageContains: []string{"case-specific field read is not exhaustive", "result.value", "result.ok == true"},
		EvidenceMin:     3,
		EvidenceOrdered: []string{
			"`result` is a union discriminated by `result.ok`",
			"`result.value` exists only for `result.ok == true`",
			"no stable guard proves `result.ok == true` before this read",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result` is a union discriminated by `result.ok`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result.value` exists only for `result.ok == true`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no stable guard proves `result.ok == true` before this read"},
			},
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; `result.value` requires `result.ok == true`",
			"--> test.lua:7:16",
			"7 |         return result.value",
			"↑ case-specific field read",
			"because:",
			"1. proven: `result` is a union discriminated by `result.ok`",
			"2. proven: `result.value` exists only for `result.ok == true`",
			"3. missing proof: no stable guard proves `result.ok == true` before this read",
			"help: Check the union case before reading this field",
		},
		RenderNotContains: []string{
			"alias.ok == true",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessKeepsAliasGuardAfterOriginalReassigned(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>, replacement: Result<string>): string
    local alias = result
    if alias.ok then
        result = replacement
        return alias.value
    else
        return ""
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want original reassignment not to invalidate guarded alias read", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsConcreteResultCaseRead(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    result = { ok = true, value = "fresh" }
    return result.value
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no result-shape exhaustiveness warning for concrete success case", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessReportsResultGuardInvalidatedBeforeRead(t *testing.T) {
	src := strings.TrimLeft(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>, replacement: Result<string>): string
    if result.ok then
        result = replacement
        return result.value
    else
        return ""
    end
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            6,
		Column:          16,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"result.value",
			"result.ok == true",
		},
		EvidenceMin: 3,
		EvidenceOrdered: []string{
			"`result` is a union discriminated by `result.ok`",
			"`result.value` exists only for `result.ok == true`",
			"no stable guard proves `result.ok == true` before this read",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result` is a union discriminated by `result.ok`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result.value` exists only for `result.ok == true`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no stable guard proves `result.ok == true` before this read"},
			},
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; `result.value` requires `result.ok == true`",
			"--> test.lua:6:16",
			"6 |         return result.value",
			"↑ case-specific field read",
			"because:",
			"1. proven: `result` is a union discriminated by `result.ok`",
			"2. proven: `result.value` exists only for `result.ok == true`",
			"3. missing proof: no stable guard proves `result.ok == true` before this read",
			"help: Check the union case before reading this field",
		},
		RenderNotContains: []string{
			"replacement.value",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsResultDiscriminantMutationBeforeRead(t *testing.T) {
	src := strings.TrimLeft(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    if result.ok then
        result.ok = false
        return result.value
    else
        return ""
    end
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            6,
		Column:          16,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"result.value",
			"result.ok == true",
		},
		EvidenceMin: 3,
		EvidenceOrdered: []string{
			"`result` is a union discriminated by `result.ok`",
			"`result.value` exists only for `result.ok == true`",
			"no stable guard proves `result.ok == true` before this read",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result` is a union discriminated by `result.ok`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result.value` exists only for `result.ok == true`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no stable guard proves `result.ok == true` before this read"},
			},
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; `result.value` requires `result.ok == true`",
			"--> test.lua:6:16",
			"6 |         return result.value",
			"↑ case-specific field read",
			"because:",
			"1. proven: `result` is a union discriminated by `result.ok`",
			"2. proven: `result.value` exists only for `result.ok == true`",
			"3. missing proof: no stable guard proves `result.ok == true` before this read",
			"help: Check the union case before reading this field",
		},
		RenderNotContains: []string{
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsResultAliasDiscriminantMutationBeforeRead(t *testing.T) {
	src := strings.TrimLeft(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    local alias = result
    if result.ok then
        alias.ok = false
        return result.value
    else
        return ""
    end
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            7,
		Column:          16,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"result.value",
			"result.ok == true",
		},
		EvidenceMin: 3,
		EvidenceOrdered: []string{
			"`result` is a union discriminated by `result.ok`",
			"`result.value` exists only for `result.ok == true`",
			"no stable guard proves `result.ok == true` before this read",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result` is a union discriminated by `result.ok`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`result.value` exists only for `result.ok == true`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no stable guard proves `result.ok == true` before this read"},
			},
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; `result.value` requires `result.ok == true`",
			"--> test.lua:7:16",
			"7 |         return result.value",
			"↑ case-specific field read",
			"because:",
			"1. proven: `result` is a union discriminated by `result.ok`",
			"2. proven: `result.value` exists only for `result.ok == true`",
			"3. missing proof: no stable guard proves `result.ok == true` before this read",
			"help: Check the union case before reading this field",
		},
		RenderNotContains: []string{
			"alias.ok == true",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsStructuralOkEnvelopeRead(t *testing.T) {
	result := Check(`
type Decode<T> = { ok: true, payload: T } | { ok: false, reason: string }

local function use(decoded: Decode<string>): string
    return decoded.payload
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{
			"decoded.payload",
			"decoded.ok == true",
		},
		EvidenceOrdered: []string{
			"`decoded` is a union discriminated by `decoded.ok`",
			"`decoded.payload` exists only for `decoded.ok == true`",
			"no stable guard proves `decoded.ok == true` before this read",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsGenericEnvelopePayloadRead(t *testing.T) {
	result := Check(`
type Envelope<T> = { kind: "data", payload: T } | { kind: "empty", reason: string }

local function use(env: Envelope<string>): string
    return env.payload
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"env.payload",
			"env.kind == \"data\"",
		},
		EvidenceOrdered: []string{
			"`env` is a union discriminated by `env.kind`",
			"`env.payload` exists only for `env.kind == \"data\"`",
			"no stable guard proves `env.kind == \"data\"` before this read",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsGenericEnvelopePayloadAfterKindGuard(t *testing.T) {
	result := Check(`
type Envelope<T> = { kind: "data", payload: T } | { kind: "empty", reason: string }

local function use(env: Envelope<string>): string
    if env.kind == "data" then
        return env.payload
    else
        return ""
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no envelope payload warning after kind guard", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsGenericEnvelopePayloadInOppositeCase(t *testing.T) {
	result := Check(`
type Envelope<T> = { kind: "data", payload: T } | { kind: "empty", reason: string }

local function use(env: Envelope<string>): string
    if env.kind == "empty" then
        return env.payload
    else
        return ""
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustiveness warning when the opposite case is already proven", result.Diagnostics)
	}
}
