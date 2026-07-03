package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

// A concrete non-top cast is runtime validation in this dialect: `x :: T` (or
// `x as T`) proves T at that expression on the normal path. Top-like casts
// (`any` and `unknown`) remain precision boundaries, not validation.

func TestStructuralCastRuntimeValidationSatisfiesParameter(t *testing.T) {
	result := Check(`
local function need(x: {name: string}): number return 1 end
local function f(y: any): number return need(y as {name: string}) end return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want structural cast runtime validation to satisfy parameter", result.Diagnostics)
	}
}

func TestStructuralCastRuntimeValidationSatisfiesDisjointParameter(t *testing.T) {
	result := Check(`
local function need(x: {name: string}): number return 1 end
local function f(y: number): number return need(y as {name: string}) end return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want structural cast runtime validation to satisfy parameter even from disjoint input", result.Diagnostics)
	}
}

func TestStructuralCastRuntimeValidationSatisfiesAnnotatedAssignment(t *testing.T) {
	result := Check(`
local function f(y: any): number
	local x: {name: string} = y as {name: string}
	return 1
end
return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want structural cast runtime validation to satisfy assignment", result.Diagnostics)
	}
}

func TestStructuralCastRuntimeValidationSatisfiesReturnContract(t *testing.T) {
	result := Check(`
local function f(y: any): {name: string} return y as {name: string} end return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want structural cast runtime validation to satisfy return contract", result.Diagnostics)
	}
}

func TestScalarCastRuntimeValidationSatisfiesFieldAssignment(t *testing.T) {
	result := Check(`
local function f(y: any, o: {name: string}): number
	o.name = y as string
	return 1
end
return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar cast runtime validation to satisfy string field assignment", result.Diagnostics)
	}
}

func TestScalarCastRuntimeValidationSatisfiesParameter(t *testing.T) {
	result := Check(`
local function need(s: string): number return 1 end
local function f(y: any): number
	return need(y as string)
end
return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar cast runtime validation to satisfy string parameter", result.Diagnostics)
	}
}

func TestScalarCastRuntimeValidationSatisfiesReturnContract(t *testing.T) {
	result := Check(`
local function f(y: any): string
	return y as string
end
return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar cast runtime validation to satisfy string return contract", result.Diagnostics)
	}
}

func TestOptionalScalarCastWithFallbackSatisfiesString(t *testing.T) {
	result := Check(`
local function need(s: string): number return 1 end
local function f(params: any): number
	local body = (params.body as string?) or ""
	return need(body)
end
return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want optional scalar cast plus string fallback to satisfy string parameter", result.Diagnostics)
	}
}

func TestAnyFieldFallbackDoesNotValidateString(t *testing.T) {
	result := Check(`
local function need(s: string): number return 1 end
local function f(params: any): number
	local body = params.body or ""
	return need(body)
end
return f
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            5,
		MessageContains: []string{
			"argument 1 (body)",
			"comes from any/unknown",
			"no proof shows it is string",
		},
	})
}

func TestTableEncodeOrAnyFallbackDoesNotValidateString(t *testing.T) {
	result := Check(`
local prompt = {}
function prompt.text(content: string): () end

local json = {}
function json.encode(value: any): string return "" end

local function clean(value: any): any
	return value
end

local function f(raw: any): ()
	local cleaned = clean(raw)
	prompt.text(type(cleaned) == "table" and json.encode(cleaned) or cleaned)
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            14,
		MessageContains: []string{
			"argument 1",
			"unknown",
			"not string",
		},
	})
}

func TestUnknownCastDoesNotValidateString(t *testing.T) {
	result := Check(`
local function need(s: string): number return 1 end
local function f(value: any): number
	return need(value :: unknown)
end
return f
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		MessageContains: []string{
			"argument 1",
			"any",
			"not string",
		},
	})
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
