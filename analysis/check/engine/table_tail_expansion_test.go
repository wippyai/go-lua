package engine_test

import "testing"

// A call in the final positional slot of a table constructor fills the array
// part with exactly the values its callee's contract names. The constructor is
// therefore a complete inventory, and both the element reads and the whole
// container annotation are proven from it.
func TestTableTailCallFillsTheExactArraySlots(t *testing.T) {
	diagnostics := checkModules(t, []string{"main"}, map[string]string{"main": `
local function mk(name: string): string
    return name
end

local xs = { mk("a"), mk("b") }
local first: string = xs[1]
local second: string = xs[2]
local all: {string} = { mk("a"), mk("b") }
print(first, second, #all)
`})
	if len(diagnostics) != 0 {
		t.Fatalf("expected a proven constructor inventory, got:\n%s", diagnosticLines(diagnostics))
	}
}

// A tail call whose contract names no result contributes no slot, so the
// constructor is the empty array its prefix already states.
func TestTableTailWithoutResultsLeavesTheConstructorEmpty(t *testing.T) {
	diagnostics := checkModules(t, []string{"main"}, map[string]string{"main": `
local function nothing() end

local xs: {string} = { nothing() }
print(#xs)
`})
	if len(diagnostics) != 0 {
		t.Fatalf("expected an empty proven constructor, got:\n%s", diagnosticLines(diagnostics))
	}
}

// Only the head slot of a multi-result tail has a bound value term. The further
// slots stay unknown, so a homogeneous element contract over them is not proven
// even though the constructor's index set is exact.
func TestTableTailFurtherResultsDoNotProveAnElementContract(t *testing.T) {
	diagnostics := checkModules(t, []string{"main"}, map[string]string{"main": `
local function pair(): (string, number)
    return "x", 1
end

local slots = { pair() }
local head: string = slots[1]
local all: {string} = { pair() }
print(head, #all)
`})
	if !hasDiagnostic(diagnostics, 8, "lint.claim.unproven", `claim "string[]" is not proven`) {
		t.Fatalf("expected the second result slot to leave the element contract unproven, got:\n%s", diagnosticLines(diagnostics))
	}
}

// An open tail with no contract to size it keeps the literal's broad table
// kind: nothing may be concluded about the slots it fills.
func TestTableTailWithoutACalleeContractStaysUnproven(t *testing.T) {
	diagnostics := checkModules(t, []string{"main"}, map[string]string{"main": `
local function forward(opaque: any): {string}
    local xs: {string} = { opaque() }
    return xs
end
print(forward(nil))
`})
	if !hasDiagnostic(diagnostics, 3, "lint.claim.unproven", `claim "string[]" is not proven`) {
		t.Fatalf("expected an unsized tail to stay unproven, got:\n%s", diagnosticLines(diagnostics))
	}
}

// A callback literal states its parameters but not its result. The body under
// the boundary that already evaluates it determines that result, so a generic
// application binds its callback type argument without an annotation.
func TestUnannotatedCallbackResultBindsAGenericTypeArgument(t *testing.T) {
	diagnostics := checkModules(t, []string{"main"}, map[string]string{"main": `
type Event = { kind: "metric" | "log", name: string }

local function map<T, U>(items: {T}, fn: (T) -> U): {U}
    local out: {U} = {}
    for _, item in ipairs(items) do
        table.insert(out, fn(item))
    end
    return out
end

local function event_for(name: string): Event
    local event: Event = { kind = "metric", name = name }
    return event
end

local names = { "latency", "errors" }
local events: {Event} = map(names, function(name: string)
    return event_for(name)
end)
print(#events)
`})
	if len(diagnostics) != 0 {
		t.Fatalf("expected the callback result to bind U, got:\n%s", diagnosticLines(diagnostics))
	}
}

// The derived callback result is a contract, not a permission: an annotation
// that disagrees with it is refuted by the exact type the body produces.
func TestUnannotatedCallbackResultRefutesADisagreeingAnnotation(t *testing.T) {
	diagnostics := checkModules(t, []string{"main"}, map[string]string{"main": `
local function map<T, U>(items: {T}, fn: (T) -> U): {U}
    local out: {U} = {}
    for _, item in ipairs(items) do
        table.insert(out, fn(item))
    end
    return out
end

local names = { "latency", "errors" }
local lengths: {string} = map(names, function(name: string)
    return #name
end)
print(#lengths)
`})
	if !hasDiagnostic(diagnostics, 11, "type.assignment", "cannot assign lengths because it is integer[], not string[]") {
		t.Fatalf("expected the derived integer result to refute the string[] annotation, got:\n%s", diagnosticLines(diagnostics))
	}
}
