package engine_test

import "testing"

// TestInferredReturnCarriesTheReturnedLocalDeclaration states what an inferred
// return exports: the declaration the returned local was checked against. The
// literal one constructor produced omits the optional member entirely, so a
// consumer reading the literal instead of the declaration receives no contract
// at all.
func TestInferredReturnCarriesTheReturnedLocalDeclaration(t *testing.T) {
	diagnostics := checkSource(t, `type Event = {kind: string, payload: string?}

local function inferred()
    local e: Event = {kind = "metric", payload = nil}
    return e
end

local value: Event = inferred()
local elements: {Event} = { inferred() }
return value.kind
`)
	if len(diagnostics) != 0 {
		t.Fatalf("an inferred return dropped its returned local's declaration:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestInferredCallableReturnCarriesTheReturnedLocalDeclaration extends the same
// contract one boundary out: the callable a body returns carries the result it
// derived, so calling it reaches the declaration rather than Top.
func TestInferredCallableReturnCarriesTheReturnedLocalDeclaration(t *testing.T) {
	diagnostics := checkSource(t, `type Event = {kind: string, payload: string?}

local function make()
    return function()
        local e: Event = {kind = "metric", payload = nil}
        return e
    end
end

local value: Event = (make())()
return value.kind
`)
	if len(diagnostics) != 0 {
		t.Fatalf("an inferred callable return dropped its returned local's declaration:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestDeclaredElementSurvivesAKeyTheLoopCouldNotResolve keeps the container
// half: the declaration types every slot the body fills, so a store at an
// unresolved key leaves the declared element rather than a synthesized keyed
// component that refutes it.
func TestDeclaredElementSurvivesAKeyTheLoopCouldNotResolve(t *testing.T) {
	diagnostics := checkSource(t, `local function collect(source: {number}): {number}
    local out: {number} = {}
    for index, value in ipairs(source) do
        out[index] = value
    end
    return out
end

local collected: {number} = collect({1, 2, 3})
return collected
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a declared container was refuted by the shape its own loop produced:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestUnannotatedReturnedLocalIsNotGivenADeclaration is the fail-closed source
// counterpart: nothing checked the literal, so its own shape is what a consumer
// reads and a wider claim about it stays unproven.
func TestUnannotatedReturnedLocalIsNotGivenADeclaration(t *testing.T) {
	diagnostics := checkSource(t, `local function unannotated()
    local e = {kind = "metric"}
    return e
end

local produced = unannotated()
local kind: number = produced.kind
return kind
`)
	if len(diagnostics) != 1 {
		t.Fatalf("an unannotated literal was not read as its own shape:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestCastDoesNotBecomeAReturnedTermDeclaration keeps claims out of the lane: a
// cast is the reader's word, not a checked contract, so it publishes no result.
func TestCastDoesNotBecomeAReturnedTermDeclaration(t *testing.T) {
	diagnostics := checkSource(t, `type Event = {kind: string, payload: string?}

local function casted(raw: any)
    local e = raw :: Event
    return e
end

local laundered: Event = casted(nil)
return laundered
`)
	if len(diagnostics) == 0 {
		t.Fatal("a cast became the declaration a returned term publishes")
	}
}

// TestSupersededCallableDoesNotAnswerForItsReplacement pins the rebinding: the
// derived contract belongs to the closure the allocation produced, and a term
// rebound to another callable must not keep answering with it.
func TestSupersededCallableDoesNotAnswerForItsReplacement(t *testing.T) {
	diagnostics := checkSource(t, `type Event = {kind: string, payload: string?}

local produce = function()
    local e: Event = {kind = "metric", payload = nil}
    return e
end
produce = function(...)
    return ...
end

local replaced: Event = produce(1)
return replaced
`)
	if len(diagnostics) == 0 {
		t.Fatal("a superseded closure's derived result answered for the callable that replaced it")
	}
}

// TestReturnedFormalKeepsTheCallerArgumentPrecision keeps the boundary
// declaration out of the lane: a formal's contract describes what a caller may
// supply, and the caller's own argument remains the more precise authority.
func TestReturnedFormalKeepsTheCallerArgumentPrecision(t *testing.T) {
	diagnostics := checkSource(t, `type Box = {v: number}

local function identity(b: Box)
    return b
end

local held = identity({v = 1})
local read: number = held.v
return read
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a returned formal lost the caller's own argument:\n%s", diagnosticSummaries(diagnostics))
	}
}
