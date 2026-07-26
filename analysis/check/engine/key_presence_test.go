package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

// TestKeyedIterationProvesTheEnumeratedSlotPresent pins the base of the key
// presence lane: pairs visits the slots a table holds, so the map read at the
// key it binds is the declared element without Lua's missing-slot nil.
func TestKeyedIterationProvesTheEnumeratedSlotPresent(t *testing.T) {
	diagnostics := checkSource(t, `local counts: {[string]: number} = {}
for name in pairs(counts) do
    local hits: number = counts[name]
end
`)
	if len(diagnostics) != 0 {
		t.Fatalf("enumerated key was not proven present:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestKeyedIterationPublishesItsPresenceRow pins the lane's vocabulary. The
// relation is a published fact keyed by the container's heap subject and the
// term the loop bound the key to, so every consumer reads the same row.
func TestKeyedIterationPublishesItsPresenceRow(t *testing.T) {
	result, err := engine.Check(`local counts: {[string]: number} = {}
for name in pairs(counts) do
    local hits: number = counts[name]
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	rows := 0
	for _, fact := range result.ValueFacts {
		if strings.HasPrefix(fact.Key, "heap/key-presence/identity/") && string(fact.Value) == "proven" {
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("keyed iteration published %d presence rows, want exactly one; facts = %#v", rows, result.ValueFacts)
	}
}

// TestKeyedIterationPresenceNamesOneContainerAndOneKey pins both directions of
// the relation: another key of the same map and another map at the same key are
// ordinary optional reads.
func TestKeyedIterationPresenceNamesOneContainerAndOneKey(t *testing.T) {
	otherKey := checkSource(t, `local counts: {[string]: number} = {}
local elsewhere = tostring(1)
for name in pairs(counts) do
    local hits: number = counts[elsewhere]
end
`)
	if !strings.Contains(diagnosticSummaries(otherKey), "may be nil") {
		t.Fatalf("a key the iteration never produced was proven present:\n%s", diagnosticSummaries(otherKey))
	}
	otherTable := checkSource(t, `local counts: {[string]: number} = {}
local mirror: {[string]: number} = {}
for name in pairs(counts) do
    local hits: number = mirror[name]
end
`)
	if !strings.Contains(diagnosticSummaries(otherTable), "may be nil") {
		t.Fatalf("a table the iteration never enumerated was proven present:\n%s", diagnosticSummaries(otherTable))
	}
}

// TestKeyedIterationPresenceDropsOnAnyLaterWrite pins the invalidation. A
// dynamic write publishes the shared revocation; a static write names its slot
// exactly and that slot may be the enumerated key, so it drops the proof alike.
func TestKeyedIterationPresenceDropsOnAnyLaterWrite(t *testing.T) {
	for _, item := range []struct{ name, source string }{
		{"dynamic write", `local counts: {[string]: number} = {}
local elsewhere = tostring(1)
for name in pairs(counts) do
    counts[elsewhere] = nil
    local hits: number = counts[name]
end
`},
		{"static write", `local counts: {[string]: number} = {}
for name in pairs(counts) do
    counts.alpha = nil
    local hits: number = counts[name]
end
`},
		{"write through an alias", `local counts: {[string]: number} = {}
local elsewhere = tostring(1)
for name in pairs(counts) do
    local same = counts
    same[elsewhere] = nil
    local hits: number = counts[name]
end
`},
		{"write inside a callee", `local counts: {[string]: number} = {}
local function clear(m: {[string]: number})
    m.alpha = nil
end
for name in pairs(counts) do
    clear(counts)
    local hits: number = counts[name]
end
`},
		{"rebound key", `local counts: {[string]: number} = {}
for name in pairs(counts) do
    name = tostring(2)
    local hits: number = counts[name]
end
`},
	} {
		diagnostics := checkSource(t, item.source)
		if !strings.Contains(diagnosticSummaries(diagnostics), "may be nil") {
			t.Fatalf("%s kept the key presence proof:\n%s", item.name, diagnosticSummaries(diagnostics))
		}
	}
}

// TestKeyedIterationPresenceDropsWithoutABodyToRead pins the escape arm. A
// callee the analysis never evaluated may have cleared the enumerated slot and
// no write of its own was projected back, while a callee whose body it does
// read keeps the proof when that body writes nothing.
func TestKeyedIterationPresenceDropsWithoutABodyToRead(t *testing.T) {
	opaque := checkSource(t, `local counts: {[string]: number} = {}
local sink: fun(m: {[string]: number})
for name in pairs(counts) do
    sink(counts)
    local hits: number = counts[name]
end
`)
	if !strings.Contains(diagnosticSummaries(opaque), "may be nil") {
		t.Fatalf("a callee with no body kept the key presence proof:\n%s", diagnosticSummaries(opaque))
	}
	evaluated := checkSource(t, `local counts: {[string]: number} = {}
local function peek(m: {[string]: number}) end
for name in pairs(counts) do
    peek(counts)
    local hits: number = counts[name]
end
`)
	if len(evaluated) != 0 {
		t.Fatalf("an evaluated callee that writes nothing dropped the proof:\n%s", diagnosticSummaries(evaluated))
	}
}

// TestKeyedIterationPresenceDropsUnderAMetatable pins the read side: an index
// metamethod answers a read the raw slot never held, so enumerating a key
// proves nothing about what the read returns.
func TestKeyedIterationPresenceDropsUnderAMetatable(t *testing.T) {
	diagnostics := checkSource(t, `local shadowed: {[string]: number} = {}
setmetatable(shadowed, { __index = function(_, _) return nil end })
for name in pairs(shadowed) do
    local hits: number = shadowed[name]
end
`)
	if !strings.Contains(diagnosticSummaries(diagnostics), "may be nil") {
		t.Fatalf("a metatable kept the key presence proof:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestDeclaredMapAnswersAComputedKey pins the read this lane refines: a map's
// declaration types every slot alike, so a key the analysis cannot resolve
// exactly still reads the declared element with the missing-slot nil.
func TestDeclaredMapAnswersAComputedKey(t *testing.T) {
	diagnostics := checkSource(t, `local counts: {[string]: number} = {}
local key = tostring(1)
local hits: number = counts[key]
`)
	if !strings.Contains(diagnosticSummaries(diagnostics), "may be nil") {
		t.Fatalf("a declared map did not answer a computed key:\n%s", diagnosticSummaries(diagnostics))
	}
	mismatch := checkSource(t, `local counts: {[string]: number} = {}
local key = tostring(1)
local hits: string? = counts[key]
`)
	if len(mismatch) == 0 {
		t.Fatalf("the declared element was not the answer: a string claim was admitted")
	}
}
