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
	if hasPublished(member, "type.return.contract", "body") {
		t.Fatalf("any member into an any? field must not be refuted: %#v", member.PublishedDiagnostics)
	}

	concrete := checkBoundary(t, `type Response = {status: number, body: string}
local function f(body: any?): Response
    return {status = 200, body = body}
end
return f`)
	// The refutation is stated at the member the declaration names.
	if !hasPublished(concrete, "type.return.contract", "returned value 1.body comes from any/unknown") {
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

// TestClosedAnyCaptureBodyIsDecidedAtAllocation pins the closed-any capture
// boundary: a body whose every formal is declared any takes no argument a
// caller can refine, and a sealed capture environment holds the same values at
// every later invocation, so its contracts are decided where it is allocated.
// A recurrence in its own control flow belongs to that body, not to a caller.
func TestClosedAnyCaptureBodyIsDecidedAtAllocation(t *testing.T) {
	result, err := engine.Check(`local helper = {}
function helper.count(): number
    return 1
end
type Report = { status: string, message: string }
local function run(options: any?): Report
    for index = 1, helper.count() do
        return { status = "error", message = index }
    end
    return { status = "ok", message = "done" }
end
return { run = run }`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(result, "type.return.contract", "returned value 1 is table, not") {
		t.Fatalf("a closed-any capture body owes its return contract: %#v", result.PublishedDiagnostics)
	}
}

// TestMutableCaptureLeavesClosedAnyBodyDormant pins the other half: a capture a
// later write can rebind is not sealed, so the value resolved at allocation is
// not the value every invocation observes and the body stays dormant.
func TestMutableCaptureLeavesClosedAnyBodyDormant(t *testing.T) {
	result, err := engine.Check(`local limit = 1
type Report = { status: string, message: string }
local function run(options: any?): Report
    return { status = "error", message = limit }
end
limit = 2
return { run = run }`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if hasPublished(result, "type.return.contract", "returned value 1") {
		t.Fatalf("an unsealed environment leaves the body dormant: %#v", result.PublishedDiagnostics)
	}
}

// TestCallThroughAnyBoundaryReturnsTheBoundary pins that a call no contract
// resolves, dispatched through an any/unknown value, publishes that boundary as
// its result. Top would state absence of information where the boundary is the
// published fact, and every obligation any owes would be discharged by it.
func TestCallThroughAnyBoundaryReturnsTheBoundary(t *testing.T) {
	method, err := engine.Check(`type Report = { status: string, message: string }
local function run(source: any): Report
    local result = source:load()
    return { status = "error", message = result.detail }
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(method, "type.return.contract", "returned value 1.message comes from any/unknown") {
		t.Fatalf("a method dispatched through any returns any: %#v", method.PublishedDiagnostics)
	}

	// A callee with a resolved contract keeps that contract: the boundary rule
	// completes an unresolved slot and never displaces a published one.
	resolved, err := engine.Check(`type Report = { status: string, message: string }
local function detail(): string
    return "text"
end
local function run(source: any): Report
    return { status = "error", message = detail() }
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if hasPublished(resolved, "type.return.contract", "comes from any/unknown") {
		t.Fatalf("a resolved callee contract survives the boundary rule: %#v", resolved.PublishedDiagnostics)
	}
}
