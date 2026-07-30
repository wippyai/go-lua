package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDiscriminatedUnionExhaustivenessHandlesResultShape(t *testing.T) {
	src := strings.TrimLeft(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    if result.ok then
        return result.value
    end
    return ""
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
		Column:          8,
		MessageContains: []string{
			"discriminated union handling is not exhaustive",
			"result.ok == false",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"branch chain checks discriminant `result.ok`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases: `result.ok == false`, `result.ok == true`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"handled cases: `result.ok == true`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing cases: `result.ok == false`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no default branch handles the remaining union cases"},
			},
		},
		LabelContains: []string{"union case check"},
		HelpContains:  []string{"Handle each missing case", "else branch"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: discriminated union handling is not exhaustive; missing case: ` + "`result.ok == false`" + `
 --> test.lua:4:8
  |
4 |     if result.ok then
  |        ↑ union case check

because:
  1. proven: branch chain checks discriminant ` + "`result.ok`" + `
  2. proven: possible cases: ` + "`result.ok == false`, `result.ok == true`" + `
  3. proven: handled cases: ` + "`result.ok == true`" + `
  4. missing proof: missing cases: ` + "`result.ok == false`" + `
  5. missing proof: no default branch handles the remaining union cases

help: Handle each missing case, or add an else branch when a fallback is valid.`
	assertRenderedEqual(t, rendered, want)
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

func TestDiscriminatedUnionExhaustivenessAcceptsNegatedGuardObjectReturn(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function map<T, U>(r: Result<T>, f: (T) -> U): Result<U>
    if not r.ok then
        return { ok = false, error = r.error }
    end
    return { ok = true, value = f(r.value) }
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want negated ok guard to prove error field in object return", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsNegatedGuardCallResultObjectReturn(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function make(): Result<number>
    return { ok = false, error = "bad" }
end

local function map<U>(f: (number) -> U): Result<U>
    local r: Result<number> = make()
    if not r.ok then
        return { ok = false, error = r.error }
    end
    return { ok = true, value = f(r.value) }
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want negated ok guard to prove call-result error field in object return", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsNegatedGuardCallbackResultObjectReturn(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }
type Validator = (number) -> Result<number>

local function map<U>(validators: {Validator}, f: (number) -> U): Result<U>
    local current = 1
    for _, validator in ipairs(validators) do
        local r: Result<number> = validator(current)
        if not r.ok then
            return { ok = false, error = r.error }
        end
        current = r.value
    end
    return { ok = true, value = f(current) }
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want negated ok guard to prove callback-result error field in object return", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsNegatedGuardOptionalSuccessPayloadObjectReturn(t *testing.T) {
	result := Check(`
type AppError = { code: string, message: string, retryable: boolean }
type StepResult = { ok: true, value: string? } | { ok: false, error: AppError }
type RunResult = { ok: true, value: string? } | { ok: false, error: AppError }
type Step = () -> StepResult

local function run(steps: {Step}): RunResult
    for _, step in ipairs(steps) do
        local r: StepResult = step()
        if not r.ok then
            return { ok = false, error = r.error }
        end
    end
    return { ok = true, value = nil }
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want negated ok guard to prove record error field with optional success payload", result.Diagnostics)
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

func TestResultValueAliasPreservesShapeThroughObjectReturnAndCallback(t *testing.T) {
	result := Check(`
type Response = { status: integer, body: string, headers: {[string]: string} }
type ResponseResult = { ok: true, value: Response } | { ok: false, error: string }
type Decorator = (string) -> string

local function build(handler: () -> ResponseResult, decorator: Decorator?): () -> ResponseResult
    return function(): ResponseResult
        local response_result = handler()
        if not response_result.ok then
            return response_result
        end
        if decorator then
            local response = response_result.value
            return {
                ok = true,
                value = {
                    status = response.status,
                    body = decorator(response.body),
                    headers = response.headers,
                },
            }
        end
        return response_result
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want result.value alias shape preserved through returned object", result.Diagnostics)
	}
}

func TestResultValueAliasPreservesShapeThroughCapturedRecordFieldCallbacks(t *testing.T) {
	result := Check(`
type Response = { status: integer, body: string, headers: {[string]: string} }
type ResponseResult = { ok: true, value: Response } | { ok: false, error: string }
type Decorator = (string) -> string
type Builder = {
    handler: () -> ResponseResult,
    decorator: Decorator?,
}

local function build(self: Builder): () -> ResponseResult
    local handler = self.handler
    local decorator = self.decorator
    return function(): ResponseResult
        local response_result = handler()
        if not response_result.ok then
            return response_result
        end
        if decorator then
            local response = response_result.value
            return {
                ok = true,
                value = {
                    status = response.status,
                    body = decorator(response.body),
                    headers = response.headers,
                },
            }
        end
        return response_result
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want captured record-field callbacks to preserve result.value shape", result.Diagnostics)
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
	src := strings.TrimLeft(`
type Decode<T> = { ok: true, payload: T } | { ok: false, reason: string }

local function use(decoded: Decode<string>): string
    return decoded.payload
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
			"decoded.payload",
			"decoded.ok == true",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`decoded` is a union discriminated by `decoded.ok`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`decoded.payload` exists only for `decoded.ok == true`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no stable guard proves `decoded.ok == true` before this read"},
			},
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; ` + "`decoded.payload`" + ` requires ` + "`decoded.ok == true`" + `
 --> test.lua:4:12
  |
4 |     return decoded.payload
  |            ↑ case-specific field read

because:
  1. proven: ` + "`decoded`" + ` is a union discriminated by ` + "`decoded.ok`" + `
  2. proven: ` + "`decoded.payload`" + ` exists only for ` + "`decoded.ok == true`" + `
  3. missing proof: no stable guard proves ` + "`decoded.ok == true`" + ` before this read

help: Check the union case before reading this field, or return from the opposite case before continuing.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessReportsGenericEnvelopePayloadRead(t *testing.T) {
	src := strings.TrimLeft(`
type Envelope<T> = { kind: "data", payload: T } | { kind: "empty", reason: string }

local function use(env: Envelope<string>): string
    return env.payload
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
			"env.payload",
			"env.kind == \"data\"",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`env` is a union discriminated by `env.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`env.payload` exists only for `env.kind == \"data\"`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no stable guard proves `env.kind == \"data\"` before this read"},
			},
		},
		LabelContains: []string{"case-specific field read"},
		HelpContains:  []string{"Check the union case before reading this field"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: case-specific field read is not exhaustive; ` + "`env.payload`" + ` requires ` + "`env.kind == \"data\"`" + `
 --> test.lua:4:12
  |
4 |     return env.payload
  |            ↑ case-specific field read

because:
  1. proven: ` + "`env`" + ` is a union discriminated by ` + "`env.kind`" + `
  2. proven: ` + "`env.payload`" + ` exists only for ` + "`env.kind == \"data\"`" + `
  3. missing proof: no stable guard proves ` + "`env.kind == \"data\"`" + ` before this read

help: Check the union case before reading this field, or return from the opposite case before continuing.`
	assertRenderedEqual(t, rendered, want)
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
