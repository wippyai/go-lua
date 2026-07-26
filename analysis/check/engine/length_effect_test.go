package engine_test

import (
	"strings"
	"testing"
)

// TestEmptyConstructorHasProvenZeroLength pins the base of the sequence
// cardinality lane: a closed constructor with no members already proves every
// omitted field absent, so its Lua length is exactly zero.
func TestEmptyConstructorHasProvenZeroLength(t *testing.T) {
	diagnostics := checkSource(t, `local function f(): string
    local arr: {string} = {}
    local n: string = #arr
    return n
end
return f
`)
	if !strings.Contains(diagnosticSummaries(diagnostics), "it is 0") {
		t.Fatalf("empty constructor did not prove length 0:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestBorderAppendsAreCountedExactly pins that the length operator reads the
// container's current slot inventory rather than its allocation-time shape: two
// border appends fill slots 1 and 2, and the slot after them stays optional.
func TestBorderAppendsAreCountedExactly(t *testing.T) {
	diagnostics := checkSource(t, `local function f(): string
    local arr: {string} = {}
    arr[#arr + 1] = "a"
    arr[#arr + 1] = "b"
    local second: string = arr[2]
    return second
end
return f
`)
	if len(diagnostics) != 0 {
		t.Fatalf("second border append was not counted:\n%s", diagnosticSummaries(diagnostics))
	}
	past := checkSource(t, `local function f(): string
    local arr: {string} = {}
    arr[#arr + 1] = "a"
    local third: string = arr[3]
    return third
end
return f
`)
	if !strings.Contains(diagnosticSummaries(past), "arr[3]") {
		t.Fatalf("slot past the appends was proven present:\n%s", diagnosticSummaries(past))
	}
}

// TestBorderAppendLengthEffectCrossesCall pins the interprocedural form: the
// callee's border append is a write to the caller's own table, so the caller's
// index proof advances by exactly one slot per call.
func TestBorderAppendLengthEffectCrossesCall(t *testing.T) {
	diagnostics := checkSource(t, `local function push(t: {string}, v: string)
    t[#t + 1] = v
end
local function f(): string
    local xs: {string} = {}
    push(xs, "a")
    push(xs, "b")
    local second: string = xs[2]
    return second
end
return f
`)
	if len(diagnostics) != 0 {
		t.Fatalf("callee border append did not cross the call boundary:\n%s", diagnosticSummaries(diagnostics))
	}
	past := checkSource(t, `local function push(t: {string}, v: string)
    t[#t + 1] = v
end
local function f(): string
    local xs: {string} = {}
    push(xs, "a")
    local second: string = xs[2]
    return second
end
return f
`)
	if !strings.Contains(diagnosticSummaries(past), "xs[2]") {
		t.Fatalf("one call proved a slot two calls establish:\n%s", diagnosticSummaries(past))
	}
}

// TestUnnameableStoreWithholdsSequenceLength keeps the inventory honest. The
// control run proves the exact cardinality; adding a store whose slot the
// analysis cannot name leaves the inventory incomplete, so the same length is
// no longer proven.
func TestUnnameableStoreWithholdsSequenceLength(t *testing.T) {
	control := checkSource(t, `local arr: {string} = {"a", "b"}
local n: string = #arr
return n
`)
	if !strings.Contains(diagnosticSummaries(control), "it is 2") {
		t.Fatalf("control run did not prove the sealed length:\n%s", diagnosticSummaries(control))
	}
	stored := checkSource(t, `local arr: {string} = {"a", "b"}
for i = 1, 3 do
    arr[i] = "z"
end
local n: string = #arr
return n
`)
	if strings.Contains(diagnosticSummaries(stored), "it is 2") {
		t.Fatalf("length stayed proven after a store the analysis cannot place:\n%s", diagnosticSummaries(stored))
	}
}

// TestCalleeDynamicKeyStoreReachesCaller pins that a dynamic-key store is a
// store to the caller's own table whichever way it resolves. An exact slot
// advances the caller's proven length; a slot the callee cannot name leaves the
// caller's inventory incomplete, so the length is withheld instead.
func TestCalleeDynamicKeyStoreReachesCaller(t *testing.T) {
	control := checkSource(t, `local function noop(t: {string})
end
local xs: {string} = {"a", "b"}
noop(xs)
local n: string = #xs
return n
`)
	if !strings.Contains(diagnosticSummaries(control), "it is 2") {
		t.Fatalf("control run did not prove the sealed length across a call:\n%s", diagnosticSummaries(control))
	}
	exact := checkSource(t, `local function put(t: {string})
    t[3] = "z"
end
local xs: {string} = {"a", "b"}
put(xs)
local n: string = #xs
return n
`)
	if !strings.Contains(diagnosticSummaries(exact), "it is 3") {
		t.Fatalf("callee store at an exact slot did not reach the caller:\n%s", diagnosticSummaries(exact))
	}
	stored := checkSource(t, `local function fill(t: {string})
    for i = 1, 3 do
        t[i] = "z"
    end
end
local xs: {string} = {"a", "b"}
fill(xs)
local n: string = #xs
return n
`)
	if strings.Contains(diagnosticSummaries(stored), "it is 2") {
		t.Fatalf("caller length stayed proven after a callee store it cannot place:\n%s", diagnosticSummaries(stored))
	}
}
