package engine_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

func checkBoundary(t *testing.T, source string) engine.Result {
	t.Helper()
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return result
}

// TestOptionalAnyFormalIsAClosedBoundary pins that an optional any formal names
// the same boundary a bare any does. No caller can supply a value more precise
// than any, so the body is decided at its declaration and its return contract
// is checked there.
func TestOptionalAnyFormalIsAClosedBoundary(t *testing.T) {
	refuted := checkBoundary(t, `local function f(v: any?): string
    return 42
end
return f`)
	if !hasPublished(refuted, "type.return.contract", "not string") {
		t.Fatalf("optional-any formal body must be checked at its declaration: %#v", refuted.PublishedDiagnostics)
	}
}

// TestOptionalAnyTargetTakesAnUnvalidatedAny pins that an optional any slot is
// a gradual declaration, not a concrete contract. It accepts an unvalidated any
// exactly as the bare form does, in a record member as well as in a local.
func TestOptionalAnyTargetTakesAnUnvalidatedAny(t *testing.T) {
	member := checkBoundary(t, `type Response = {status: number, body: any?}
local function f(body: any?): Response
    return {status = 200, body = body}
end
return f`)
	if hasPublished(member, "type.return.contract", "not ") {
		t.Fatalf("any member into an any? field must not be refuted: %#v", member.PublishedDiagnostics)
	}

	concrete := checkBoundary(t, `type Response = {status: number, body: string}
local function f(body: any?): Response
    return {status = 200, body = body}
end
return f`)
	if !hasPublished(concrete, "type.return.contract", "not ") {
		t.Fatalf("any member into a string field must stay refuted: %#v", concrete.PublishedDiagnostics)
	}
}

// TestRuntimeTypeTestValidatesACallArgument pins that Lua's own type() proof is
// the any boundary's validator in argument position, exactly as it already is
// for an assignment and a return. Without the proof the argument stays refuted.
func TestRuntimeTypeTestValidatesACallArgument(t *testing.T) {
	validated := checkBoundary(t, `local function need(s: string): number return 1 end
local x: any = 1
if type(x) == "string" then
    need(x)
end`)
	if hasPublished(validated, "type.call.direct.argument_type", "is any, not string") {
		t.Fatalf("a proven string edge validates the argument: %#v", validated.PublishedDiagnostics)
	}

	unvalidated := checkBoundary(t, `local function need(s: string): number return 1 end
local x: any = 1
need(x)`)
	if !hasPublished(unvalidated, "type.call.direct.argument_type", "is any, not string") {
		t.Fatalf("an unvalidated any argument stays refuted: %#v", unvalidated.PublishedDiagnostics)
	}

	mismatched := checkBoundary(t, `local function need(s: string): number return 1 end
local x: any = 1
if type(x) == "number" then
    need(x)
end`)
	if !hasPublished(mismatched, "type.call.direct.argument_type", "is any, not string") {
		t.Fatalf("a number proof decides nothing about a string parameter: %#v", mismatched.PublishedDiagnostics)
	}
}

// TestRuntimeTypeTestValidatesAnOptionalParameter pins that an optional
// parameter accepts the proof of the base it wraps: proving string decides
// string?, while the nil proof keeps its exact target.
func TestRuntimeTypeTestValidatesAnOptionalParameter(t *testing.T) {
	validated := checkBoundary(t, `local function need(s: string?): number return 1 end
local x: any = 1
if type(x) == "string" then
    need(x)
end`)
	if hasPublished(validated, "type.call.direct.argument_type", "is any, not string?") {
		t.Fatalf("a proven string edge validates a string? parameter: %#v", validated.PublishedDiagnostics)
	}

	unvalidated := checkBoundary(t, `local function need(s: string?): number return 1 end
local x: any = 1
need(x)`)
	if !hasPublished(unvalidated, "type.call.direct.argument_type", "is any, not string?") {
		t.Fatalf("an unvalidated any argument stays refuted against string?: %#v", unvalidated.PublishedDiagnostics)
	}
}
