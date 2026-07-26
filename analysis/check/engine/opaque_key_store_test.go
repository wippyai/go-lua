package engine_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

// TestClosedInventoryWithoutOpaqueStoreStillProvesAbsence pins the control: a
// closed constructor that was never stored through at an unresolved key keeps
// its closure, so an omitted member is proven absent.
func TestClosedInventoryWithoutOpaqueStoreStillProvesAbsence(t *testing.T) {
	diagnostics := checkSource(t, `local sealed = { present = 1 }
local missing: nil = sealed.absent
local wrong: number = sealed.absent
`)
	summary := diagnosticSummaries(diagnostics)
	if strings.Contains(summary, "main.lua:2") {
		t.Fatalf("a closed constructor stopped proving an omitted member absent:\n%s", summary)
	}
	if !strings.Contains(summary, "main.lua:3") {
		t.Fatalf("a proven-absent member was accepted as number:\n%s", summary)
	}
}

// TestOpaqueKeyStoreRemovesComputedReadAbsence pins the hole the store opens.
// The loop stores at a key drawn from a string array, so "x" is one of the
// slots it can have written and the read is not nil.
func TestOpaqueKeyStoreRemovesComputedReadAbsence(t *testing.T) {
	diagnostics := checkSource(t, `local keys: {string} = {}
local suites = {}
for _, key in ipairs(keys) do suites[key] = 1 end
local probe = tostring(1)
local absent: nil = suites[probe]
`)
	if len(diagnostics) == 0 {
		t.Fatal("a table stored through at an unresolved key was still proven empty at a computed read")
	}
}

// TestOpaqueKeyStoreRemovesSpelledReadAbsence pins the same obligation for a
// spelled read: Lua indexes a dotted name by its string key, so the store's
// key can be exactly that name.
func TestOpaqueKeyStoreRemovesSpelledReadAbsence(t *testing.T) {
	for _, item := range []struct{ name, read string }{
		{"static string index", `suites["x"]`},
		{"dotted member", `suites.y`},
	} {
		t.Run(item.name, func(t *testing.T) {
			diagnostics := checkSource(t, `local keys: {string} = {}
local suites = {}
for _, key in ipairs(keys) do suites[key] = 1 end
local absent: nil = `+item.read+`
`)
			if len(diagnostics) == 0 {
				t.Fatalf("a table stored through at an unresolved key was still proven empty at %s", item.read)
			}
		})
	}
}

// TestOpaqueKeyStoreRemovesAbsenceAcrossTheCallBoundary pins the same shape
// where the container is produced by a callee: the store belongs to the return
// authority, so the caller reads the answer the producing body reads.
func TestOpaqueKeyStoreRemovesAbsenceAcrossTheCallBoundary(t *testing.T) {
	diagnostics := checkSource(t, `local function build(source: {string})
    local out = {}
    for _, key in ipairs(source) do out[key] = 1 end
    return out
end
local keys: {string} = {}
local returned = build(keys)
local computed: nil = returned["x"]
local spelled: nil = returned.y
`)
	summary := diagnosticSummaries(diagnostics)
	for _, line := range []string{"main.lua:8", "main.lua:9"} {
		if !strings.Contains(summary, line) {
			t.Fatalf("a returned table stored through at an unresolved key was still proven empty at %s:\n%s", line, summary)
		}
	}
}

// TestOpaqueKeyStoreRemovesAbsenceThroughTransportedContainers pins the two
// remaining routes a callee reaches the caller's table by. Both republish the
// store, so the caller's own read stops proving absence.
func TestOpaqueKeyStoreRemovesAbsenceThroughTransportedContainers(t *testing.T) {
	for _, item := range []struct{ name, source string }{
		{"captured container", `local acc = {}
local function fill(keys: {string})
    for _, key in ipairs(keys) do acc[key] = 1 end
end
local ks: {string} = {}
fill(ks)
local absent: nil = acc["x"]
`},
		{"argument container", `local function fill(acc, keys: {string})
    for _, key in ipairs(keys) do acc[key] = 1 end
end
local target = {}
local ks: {string} = {}
fill(target, ks)
local absent: nil = target["x"]
`},
	} {
		t.Run(item.name, func(t *testing.T) {
			if len(checkSource(t, item.source)) == 0 {
				t.Fatal("a callee's unresolved-key store did not reach the caller's inventory")
			}
		})
	}
}

// TestKeyedComponentTypesTheUnnamedSlots pins the component the stores
// establish: their key type is the key domain and their value type is what
// every unnamed slot holds. Presence is not among what it states.
func TestKeyedComponentTypesTheUnnamedSlots(t *testing.T) {
	for _, item := range []struct{ name, read, want string }{
		{"computed key element", `local element: integer? = suites[probe]`, ""},
		{"spelled key element", `local element: integer? = suites["x"]`, ""},
		{"dotted key element", `local element: integer? = suites.y`, ""},
		{"presence is not proven", `local present: integer = suites[probe]`, "may be nil"},
		{"presence is not proven at a spelled key", `local present: integer = suites["x"]`, "may be nil"},
		{"another element is refuted", `local other: string? = suites[probe]`, "not string?"},
		{"another element is refuted at a spelled key", `local other: string? = suites["x"]`, "not string?"},
		{"another element is refuted at a dotted key", `local other: string? = suites.y`, "not string?"},
	} {
		t.Run(item.name, func(t *testing.T) {
			summary := diagnosticSummaries(checkSource(t, `local keys: {string} = {}
local suites = {}
for _, key in ipairs(keys) do suites[key] = 1 end
local probe = tostring(1)
`+item.read+"\n"))
			if item.want == "" {
				if summary != "" {
					t.Fatalf("the keyed component did not type the read:\n%s", summary)
				}
				return
			}
			if !strings.Contains(summary, item.want) {
				t.Fatalf("expected %q in:\n%s", item.want, summary)
			}
		})
	}
}

// TestKeyedComponentCrossesTheCallBoundary pins the component as part of the
// return authority, not merely as a local read answer.
func TestKeyedComponentCrossesTheCallBoundary(t *testing.T) {
	summary := diagnosticSummaries(checkSource(t, `local function build(source: {string})
    local out = {}
    for _, key in ipairs(source) do out[key] = 1 end
    return out
end
local keys: {string} = {}
local returned = build(keys)
local element: integer? = returned["x"]
`))
	if summary != "" {
		t.Fatalf("the keyed component did not cross the return boundary:\n%s", summary)
	}
}

// TestKeyedComponentJoinsEveryRecordedStore pins the join: two stores of
// different literal integers widen to their primitive, and a store of another
// primitive keeps both arms.
func TestKeyedComponentJoinsEveryRecordedStore(t *testing.T) {
	uniform := diagnosticSummaries(checkSource(t, `local keys: {string} = {}
local suites = {}
for _, key in ipairs(keys) do
    suites[key] = 1
    suites[key] = 2
end
local element: integer? = suites["x"]
`))
	if uniform != "" {
		t.Fatalf("a homogeneous literal family did not widen to its primitive:\n%s", uniform)
	}
	mixed := diagnosticSummaries(checkSource(t, `local keys: {string} = {}
local suites = {}
for _, key in ipairs(keys) do
    suites[key] = 1
    suites[key] = "two"
end
local element: (integer | string)? = suites["x"]
`))
	if mixed != "" {
		t.Fatalf("stores of different primitives did not join:\n%s", mixed)
	}
}

// TestKeyedComponentIsWithheldWhereItWouldNotDescribeTheContainer pins the
// fail-closed side. Each case leaves the container without a component while
// the lost closure still stands, so the read is unknown rather than absent or
// typed.
func TestKeyedComponentIsWithheldWhereItWouldNotDescribeTheContainer(t *testing.T) {
	for _, item := range []struct{ name, source string }{
		{"key outside the key domain", `local tables = { {}, {} }
local sink = {}
for _, key in ipairs(tables) do sink[key] = 1 end
`},
		{"value is gradual", `local keys: {string} = {}
local sink = {}
local opaque: any = 1
for _, key in ipairs(keys) do sink[key] = opaque end
`},
		{"container also holds a named slot", `local keys: {string} = {}
local sink = { named = "a" }
for _, key in ipairs(keys) do sink[key] = 1 end
`},
	} {
		t.Run(item.name, func(t *testing.T) {
			absent := diagnosticSummaries(checkSource(t, item.source+"local absent: nil = sink[\"x\"]\n"))
			if absent == "" {
				t.Fatal("the container was still proven empty")
			}
			typed := diagnosticSummaries(checkSource(t, item.source+"local element: integer? = sink[\"x\"]\n"))
			if typed == "" {
				t.Fatal("a component was synthesized from stores that do not describe the container")
			}
		})
	}
}

// TestOpaqueMemberWriteRecordsItsKeyAndValueTypes pins the fact vocabulary.
// The marker is one row per unresolved store, and the row carries the store's
// own key and value publications.
func TestOpaqueMemberWriteRecordsItsKeyAndValueTypes(t *testing.T) {
	result, err := engine.Check(`local keys: {string} = {}
local suites = {}
for _, key in ipairs(keys) do suites[key] = 1 end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	rows := 0
	for _, fact := range result.ValueFacts {
		family, ok := factkey.Lookup(fact.Key)
		if !ok || family.ID != factkey.FamilyHeapOpaqueMemberWrite {
			continue
		}
		rows++
		var wire struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(fact.Value, &wire); err != nil {
			t.Fatalf("unresolved-store payload does not decode: %v", err)
		}
		if !strings.HasPrefix(wire.Key, "shape/target/v1/") || !strings.HasPrefix(wire.Value, "shape/target/v1/") {
			t.Fatalf("unresolved store recorded no key/value publication: %#v", wire)
		}
	}
	if rows != 1 {
		t.Fatalf("expected exactly one unresolved-store row, got %d; facts = %#v", rows, result.ValueFacts)
	}
}

// TestKeyedComponentValueFollowsElementsAppendedThroughIt pins the feedback the
// component's value rests on. The store places an empty literal at an
// unresolved key; the appends through a read of that same slot are what decide
// the literal is an array and what it holds.
func TestKeyedComponentValueFollowsElementsAppendedThroughIt(t *testing.T) {
	const source = `type Entry = {id: string}
local entries: {Entry} = {}
local groups = {}
for _, entry in ipairs(entries) do
    groups[entry.id] = groups[entry.id] or {}
    table.insert(groups[entry.id], entry)
end
local group: %s = groups["alpha"]
`
	if diagnostics := checkSource(t, fmt.Sprintf(source, "{Entry}?")); len(diagnostics) != 0 {
		t.Fatalf("the appended element did not reach the component's value:\n%s", diagnosticSummaries(diagnostics))
	}
	// The value is the element the appends proved, not a permissive stand-in:
	// another element on the same slot is refuted against it.
	wrong := diagnosticSummaries(checkSource(t, fmt.Sprintf(source, "{string}?")))
	if !strings.Contains(wrong, "not string[]?") {
		t.Fatalf("a mismatched element claim was admitted against the component:\n%s", wrong)
	}
	// The empty literal the store placed there is no longer the answer.
	absent := diagnosticSummaries(checkSource(t, fmt.Sprintf(source, "nil")))
	if !strings.Contains(absent, "{id: string}[]?") {
		t.Fatalf("the component still answers with the store's own empty literal:\n%s", absent)
	}
}

// TestKeyedComponentValueCrossesTheCallBoundaryItWasBuiltIn pins that the
// refined component is the authority the producing body's return carries, not a
// fact only its own partition holds.
func TestKeyedComponentValueCrossesTheCallBoundaryItWasBuiltIn(t *testing.T) {
	const source = `type Entry = {id: string}
local entries: {Entry} = {}
local function group_by_id(items: {Entry})
    local built = {}
    for _, entry in ipairs(items) do
        built[entry.id] = built[entry.id] or {}
        table.insert(built[entry.id], entry)
    end
    return built
end
local returned = group_by_id(entries)
local group: %s = returned["alpha"]
`
	if diagnostics := checkSource(t, fmt.Sprintf(source, "{Entry}?")); len(diagnostics) != 0 {
		t.Fatalf("the component's refined value did not cross the return boundary:\n%s", diagnosticSummaries(diagnostics))
	}
	wrong := diagnosticSummaries(checkSource(t, fmt.Sprintf(source, "{number}?")))
	if !strings.Contains(wrong, "not number[]?") {
		t.Fatalf("a mismatched element claim was admitted across the boundary:\n%s", wrong)
	}
}

// TestKeyedComponentWithholdsWhatTheAppendsCannotAccount pins the fail-closed
// edges. Each source keeps the same store and the same read; only the account
// of what reached the slot changes, and none of them may leave the component
// standing on the store's own empty literal.
func TestKeyedComponentWithholdsWhatTheAppendsCannotAccount(t *testing.T) {
	for name, body := range map[string]string{
		"element-with-no-published-type": `    groups[entry.id] = groups[entry.id] or {}
    table.insert(groups[entry.id], unresolved_source())`,
		"slot-reached-an-unread-callee": `    groups[entry.id] = groups[entry.id] or {}
    table.insert(groups[entry.id], entry)
    unresolved_sink(groups[entry.id])`,
		"store-nothing-could-append-to": `    groups[entry.id] = 1
    table.insert(groups[entry.id], entry)`,
	} {
		t.Run(name, func(t *testing.T) {
			summary := diagnosticSummaries(checkSource(t, `type Entry = {id: string}
local entries: {Entry} = {}
local groups = {}
for _, entry in ipairs(entries) do
`+body+`
end
local group: {Entry}? = groups["alpha"]
`))
			if !strings.Contains(summary, "is not proven") {
				t.Fatalf("the component survived an append it cannot account for:\n%s", summary)
			}
		})
	}
}

// TestGuardedOpaqueKeyStoreRevokesClosureAtTheJoin pins the marker as a
// may-fact. The arm that stored at an unresolved key is one of the executions
// arriving past the decision, so the constructor no longer proves an omitted
// slot absent there.
func TestGuardedOpaqueKeyStoreRevokesClosureAtTheJoin(t *testing.T) {
	for name, source := range map[string]string{
		"acyclic-arm": `local guarded = {}
local key = tostring(1)
if key ~= "" then
    guarded[key] = 1
end
local absent: nil = guarded["x"]
`,
		"loop-body-arm": `local keys: {string} = {}
local guarded = {}
for _, key in ipairs(keys) do
    if key ~= "" then
        guarded[key] = 1
    end
end
local absent: nil = guarded["x"]
`,
	} {
		t.Run(name, func(t *testing.T) {
			if len(checkSource(t, source)) == 0 {
				t.Fatal("a store on one arm left the constructor proving an omitted slot absent")
			}
		})
	}
	// A container never stored through at an unresolved key keeps its closure.
	control := checkSource(t, `local sealed = { present = 1 }
local missing: nil = sealed.absent
`)
	if len(control) != 0 {
		t.Fatalf("a container with no unresolved-key store lost its closure:\n%s", diagnosticSummaries(control))
	}
}

// TestGuardedOpaqueKeyStoreTypesTheReadFromItsArm pins that the arm's own store
// is what the component types the read with, so the revocation is a proof about
// the slot rather than a bare unknown.
func TestGuardedOpaqueKeyStoreTypesTheReadFromItsArm(t *testing.T) {
	const source = `local guarded = {}
local key = tostring(1)
if key ~= "" then
    guarded[key] = 1
end
local read: %s = guarded["x"]
`
	if diagnostics := checkSource(t, fmt.Sprintf(source, "integer?")); len(diagnostics) != 0 {
		t.Fatalf("the arm's store did not type the read:\n%s", diagnosticSummaries(diagnostics))
	}
	for _, claim := range []string{"integer", "string?"} {
		if len(checkSource(t, fmt.Sprintf(source, claim))) == 0 {
			t.Fatalf("claim %q was admitted against the component the arm established", claim)
		}
	}
}

// TestKeyedStoreClassifiesWriteBackEdgesFromTheRemainingArms pins the store
// witness a decision's edges produce. An edge that stores back a read of the
// same container's unresolved keys leaves the slot as it found it, so the value
// the component takes is the one the other edge writes.
func TestKeyedStoreClassifiesWriteBackEdgesFromTheRemainingArms(t *testing.T) {
	const source = `type Entry = {id: string}
local entries: {Entry} = {}
local groups = {}
for _, entry in ipairs(entries) do
    groups[entry.id] = groups[entry.id] or {}
    table.insert(groups[entry.id], entry)
end
local group: %s = groups["alpha"]
`
	if diagnostics := checkSource(t, fmt.Sprintf(source, "{Entry}?")); len(diagnostics) != 0 {
		t.Fatalf("the write-back idiom lost the component's value:\n%s", diagnosticSummaries(diagnostics))
	}
	if len(checkSource(t, fmt.Sprintf(source, "{string}?"))) == 0 {
		t.Fatal("a mismatched element claim was admitted against the write-back idiom")
	}
}

// TestEnumerationBindsTheKeyedComponentItsStoresEstablished pins the authority
// an enumeration reads. The container carries no declaration, so the component
// its own unresolved-key stores establish is its key domain and element -- the
// same answer an index read of it already consumes.
func TestEnumerationBindsTheKeyedComponentItsStoresEstablished(t *testing.T) {
	const source = `type Entry = {id: string}
local entries: {Entry} = {}
local suites = {}
for _, entry in ipairs(entries) do
    suites[entry.id] = suites[entry.id] or {}
    table.insert(suites[entry.id], entry)
end
for key in pairs(suites) do
    local bound: %s = %s
end
`
	if diagnostics := checkSource(t, fmt.Sprintf(source, "{Entry}", "suites[key]")); len(diagnostics) != 0 {
		t.Fatalf("a read inside the enumeration lost the component the same read outside it consumes:\n%s", diagnosticSummaries(diagnostics))
	}
	if diagnostics := checkSource(t, fmt.Sprintf(source, "string", "key")); len(diagnostics) != 0 {
		t.Fatalf("the enumeration bound no key domain:\n%s", diagnosticSummaries(diagnostics))
	}
	// Both sides are the component's own, not permissive stand-ins.
	for _, mismatch := range [][2]string{{"number", "key"}, {"{string}", "suites[key]"}} {
		if len(checkSource(t, fmt.Sprintf(source, mismatch[0], mismatch[1]))) == 0 {
			t.Fatalf("claim %q on %s was admitted against the component", mismatch[0], mismatch[1])
		}
	}
}

// TestEnumerationOfAComponentlessContainerBindsNothing pins the fail-closed
// half: a container whose stores establish no component states nothing about
// its slots, so its enumeration binds an unknown key.
func TestEnumerationOfAComponentlessContainerBindsNothing(t *testing.T) {
	summary := diagnosticSummaries(checkSource(t, `type Entry = {id: string}
local entries: {Entry} = {}
local untyped = {}
for _, entry in ipairs(entries) do
    untyped[entry.id] = unresolved_source()
end
for key in pairs(untyped) do
    local spelled: string = key
end
`))
	if !strings.Contains(summary, "is not proven") {
		t.Fatalf("an enumeration bound a key domain no store established:\n%s", summary)
	}
}

// TestEnumeratedKeyOfAnInferredComponentProvesItsRead pins that the presence
// the enumeration establishes and the element the component states reach the
// same read: the slot the loop visits is occupied.
func TestEnumeratedKeyOfAnInferredComponentProvesItsRead(t *testing.T) {
	diagnostics := checkSource(t, `local keys: {string} = {}
local counts = {}
for _, key in ipairs(keys) do
    counts[key] = 1
end
for key in pairs(counts) do
    local hit: integer = counts[key]
end
`)
	if len(diagnostics) != 0 {
		t.Fatalf("the enumerated slot of an inferred component was not proven occupied:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestGuardedAppendReachesTheKeyedComponent pins the keyed-store families as
// may-facts. An append through a read at an unresolved key belongs to the
// container's component wherever it stands, so an arm of a decision states it
// as much as an unguarded point does.
func TestGuardedAppendReachesTheKeyedComponent(t *testing.T) {
	const source = `type Entry = {id: string}
local entries: {Entry} = {}
local groups = {}
for _, entry in ipairs(entries) do
    local key = entry.id
    if key ~= "" then
        groups[key] = groups[key] or {}
        table.insert(groups[key], entry)
    end
end
local group: %s = groups["alpha"]
`
	if diagnostics := checkSource(t, fmt.Sprintf(source, "{Entry}?")); len(diagnostics) != 0 {
		t.Fatalf("an append inside an arm did not reach the component:\n%s", diagnosticSummaries(diagnostics))
	}
	if len(checkSource(t, fmt.Sprintf(source, "{string}?"))) == 0 {
		t.Fatal("a mismatched element claim was admitted against the component the arm established")
	}
}

// TestShapePreservingMutationAccountsForItself pins the effect row a permuting
// call publishes. table.sort states a mutation that changes neither shape nor
// length, so it introduces no element the enumeration did not produce and the
// keys-of relation survives it.
func TestShapePreservingMutationAccountsForItself(t *testing.T) {
	diagnostics := checkSource(t, `local function keys_of(source: {[string]: number})
    local out = {}
    for key in pairs(source) do table.insert(out, key) end
    table.sort(out)
    return out
end
local counts: {[string]: number} = {}
local names = keys_of(counts)
for _, name in ipairs(names) do
    local hits: number = counts[name]
end
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a shape-preserving mutation withdrew the relation:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestUndeclaredFormalHoldsTheArgumentsKeyedComponent pins the instance a call
// seeds. The argument publishes no declaration and no type witness, so the
// component its own stores established is what the formal holds.
func TestUndeclaredFormalHoldsTheArgumentsKeyedComponent(t *testing.T) {
	diagnostics := checkSource(t, `type Entry = {id: string}
local entries: {Entry} = {}
local groups = {}
for _, entry in ipairs(entries) do
    groups[entry.id] = groups[entry.id] or {}
    table.insert(groups[entry.id], entry)
end
local function keys_of(t)
    local out = {}
    for key in pairs(t) do table.insert(out, key) end
    return out
end
local names = keys_of(groups)
for _, name in ipairs(names) do
    local tests: {Entry} = groups[name]
end
`)
	if len(diagnostics) != 0 {
		t.Fatalf("an undeclared formal lost the argument's component:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestReturnedKeyedContainerCarriesARevocableIdentity pins why the returned
// container needs an identity: the presence lane names a container only by one,
// and that identity is what every later write publishes its revocation under.
func TestReturnedKeyedContainerCarriesARevocableIdentity(t *testing.T) {
	const source = `type Entry = {id: string}
local function group(items: {Entry})
    local out = {}
    for _, item in ipairs(items) do
        out[item.id] = out[item.id] or {}
        table.insert(out[item.id], item)
    end
    return out
end
local function keys_of(t)
    local out = {}
    for key in pairs(t) do table.insert(out, key) end
    return out
end
local entries: {Entry} = {}
local groups = group(entries)
local names = keys_of(groups)
%s
for _, name in ipairs(names) do
    local tests: {Entry} = groups[name]
end
`
	if diagnostics := checkSource(t, fmt.Sprintf(source, "")); len(diagnostics) != 0 {
		t.Fatalf("the returned container carried no relation to its keys:\n%s", diagnosticSummaries(diagnostics))
	}
	for name, intervening := range map[string]string{
		"store-into-the-container":    `groups["added"] = {}`,
		"container-to-unread-callee":  `unresolved_sink(groups)`,
		"element-no-enumeration-made": `table.insert(names, "literal")`,
	} {
		t.Run(name, func(t *testing.T) {
			if len(checkSource(t, fmt.Sprintf(source, intervening))) == 0 {
				t.Fatal("the relation survived a change its identity must revoke")
			}
		})
	}
}
