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

// TestReturnedKeysOfContainerProvesThePresenceItRelates pins the cross-call
// half of the presence lane. The callee states the relation against its own
// formal; the application substitutes the argument, so the caller's read at an
// element of the returned array is decided against the caller's container.
func TestReturnedKeysOfContainerProvesThePresenceItRelates(t *testing.T) {
	diagnostics := checkSource(t, `local counts: {[string]: number} = {}
local function keys_of(source: {[string]: number})
    local out = {}
    for key in pairs(source) do
        table.insert(out, key)
    end
    return out
end
local names = keys_of(counts)
for _, name in ipairs(names) do
    local hits: number = counts[name]
end
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a returned array of the argument's keys did not prove its reads present:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestReturnedKeysOfContainerWithholdsWhatItCannotAccount pins the relation's
// fail-closed edges. Each source keeps the same call and the same read; only
// the fact the relation rests on is removed.
func TestReturnedKeysOfContainerWithholdsWhatItCannotAccount(t *testing.T) {
	const callee = `local counts: {[string]: number} = {}
local function keys_of(source: {[string]: number})
    local out = {}
    for key in pairs(source) do
        table.insert(out, key)
    end
    return out
end
`
	for name, body := range map[string]string{
		"another-container": `local other: {[string]: number} = {}
local names = keys_of(other)
for _, name in ipairs(names) do
    local hits: number = counts[name]
end
`,
		"container-mutated-after-the-relation": `local names = keys_of(counts)
counts["added"] = 1
for _, name in ipairs(names) do
    local hits: number = counts[name]
end
`,
		"element-no-enumeration-produced": `local names = keys_of(counts)
table.insert(names, "literal")
for _, name in ipairs(names) do
    local hits: number = counts[name]
end
`,
		"container-reached-an-unevaluated-callee": `local names = keys_of(counts)
opaque_sink(counts)
for _, name in ipairs(names) do
    local hits: number = counts[name]
end
`,
	} {
		t.Run(name, func(t *testing.T) {
			// A withheld relation leaves the read undecided. Which answer the
			// read then carries -- Lua's missing slot or no element at all --
			// depends on what else the removed fact took with it; neither admits
			// the declaration.
			diagnostics := checkSource(t, callee+body)
			if len(diagnostics) == 0 {
				t.Fatal("relation survived a fact it cannot account for")
			}
		})
	}
}

// TestKeysOfRelationRequiresAnUnwrittenFormal pins the producer's own gate. A
// callee that writes the formal it enumerated returns keys of a container it
// has itself since changed, so the relation is never stated.
func TestKeysOfRelationRequiresAnUnwrittenFormal(t *testing.T) {
	diagnostics := checkSource(t, `local counts: {[string]: number} = {}
local function keys_and_write(source: {[string]: number})
    local out = {}
    for key in pairs(source) do
        table.insert(out, key)
    end
    source.extra = 1
    return out
end
local names = keys_and_write(counts)
for _, name in ipairs(names) do
    local hits: number = counts[name]
end
`)
	if !strings.Contains(diagnosticSummaries(diagnostics), "may be nil") {
		t.Fatalf("a callee that wrote its enumerated formal still stated the relation:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestKeysOfRelationPublishesOneAccountedRowPerWrite pins the vocabulary: the
// relation is a per-write accounting row against the array's heap identity, and
// the application anchors that same row on the result it hands the caller.
func TestKeysOfRelationPublishesOneAccountedRowPerWrite(t *testing.T) {
	result, err := engine.Check(`local counts: {[string]: number} = {}
local function keys_of(source: {[string]: number})
    local out = {}
    for key in pairs(source) do
        table.insert(out, key)
    end
    return out
end
local names = keys_of(counts)
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	relation, application := 0, 0
	for _, fact := range result.ValueFacts {
		if strings.HasPrefix(fact.Key, "heap/keys-of/") {
			relation++
		}
		if strings.HasPrefix(fact.Key, "call-keys-of/") {
			application++
		}
	}
	if application != 1 {
		t.Fatalf("application published %d keys-of rows, want exactly one; facts = %#v", application, result.ValueFacts)
	}
	if relation == 0 {
		t.Fatalf("no accounted keys-of row reached the caller; facts = %#v", result.ValueFacts)
	}
}

// TestAccumulatedArraySurfacesItsMembersAtALocalRead pins the aggregate a
// locally filled array carries. An append advances the member cells rather than
// the allocation's aggregate value, so without the rebuild the return boundary
// performs, an iteration inside the filling body reads no element at all.
func TestAccumulatedArraySurfacesItsMembersAtALocalRead(t *testing.T) {
	diagnostics := checkSource(t, `local counts: {[string]: number} = {}
local names = {}
for key in pairs(counts) do
    table.insert(names, key)
end
for _, name in ipairs(names) do
    local spelled: string = name
end
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a locally accumulated array carried no element at a local read:\n%s", diagnosticSummaries(diagnostics))
	}
	// The element is the appended one, not a permissive stand-in.
	wrong := diagnosticSummaries(checkSource(t, `local counts: {[string]: number} = {}
local names = {}
for key in pairs(counts) do
    table.insert(names, key)
end
for _, name in ipairs(names) do
    local spelled: number = name
end
`))
	if !strings.Contains(wrong, "not number") {
		t.Fatalf("a mismatched element claim was admitted on the rebuilt aggregate:\n%s", wrong)
	}
}

// TestAccumulatedArraySurfaceCarriesTheKeysOfRelationLocally pins the chain the
// rebuild completes: the enumerated keys reach an array, the iteration over
// that array binds them back, and the read at one of them is the enumeration's
// own -- all inside the body that built it.
func TestAccumulatedArraySurfaceCarriesTheKeysOfRelationLocally(t *testing.T) {
	diagnostics := checkSource(t, `local counts: {[string]: number} = {}
local names = {}
for key in pairs(counts) do
    table.insert(names, key)
end
for _, name in ipairs(names) do
    local hits: number = counts[name]
end
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a locally built keys array did not prove its reads present:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestAccumulatedArraySurfaceWithholdsWhatItsMembersDoNotDescribe pins the
// rebuild's own authority. Each source keeps the same appends and the same
// read; only the account of the container's slots changes.
func TestAccumulatedArraySurfaceWithholdsWhatItsMembersDoNotDescribe(t *testing.T) {
	for name, body := range map[string]string{
		"container-reached-an-unread-callee": `for key in pairs(counts) do
    table.insert(names, key)
end
unresolved_sink(names)`,
		"store-at-an-unresolved-key": `for key in pairs(counts) do
    table.insert(names, key)
    names[key] = key
end`,
	} {
		t.Run(name, func(t *testing.T) {
			summary := diagnosticSummaries(checkSource(t, `local counts: {[string]: number} = {}
local names = {}
`+body+`
for _, name in ipairs(names) do
    local spelled: string = name
end
`))
			if !strings.Contains(summary, "is not proven") {
				t.Fatalf("the rebuild survived a slot its member cells do not describe:\n%s", summary)
			}
		})
	}
}
