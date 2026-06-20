package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

// A cast adopts its target type for inference, but it must not prove a separate
// obligation that the uncast value could not. These probes guard that an any or
// disjoint value cannot launder past a parameter, assignment, or return contract
// through a cast, while a cast used purely for local inference stays clean.

func TestCastDoesNotLaunderAnyIntoParameter(t *testing.T) {
	result := Check(`
local function need(x: {name: string}): number return 1 end
local function f(y: any): number return need(y as {name: string}) end return f
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	if got := diag.Message; got != "argument 1 (y) is any, not {name: string}" {
		t.Fatalf("message = %q, want any-not-record argument mismatch", got)
	}
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
	result := Check(`
local function f(y: any): number
	local x: {name: string} = y as {name: string}
	return 1
end
return f
`)
	requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
}

func TestCastDoesNotLaunderAnyIntoReturnContract(t *testing.T) {
	result := Check(`
local function f(y: any): {name: string} return y as {name: string} end return f
`)
	requireDiagnosticCode(t, result, diagnostics.CodeReturnContractType)
}

func TestCastDoesNotLaunderAnyIntoFieldAssignment(t *testing.T) {
	result := Check(`
local function f(y: any, o: {name: string}): number
	o.name = y as string
	return 1
end
return f
`)
	requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
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
