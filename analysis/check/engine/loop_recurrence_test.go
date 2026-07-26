package engine_test

import (
	"strings"
	"testing"
)

// TestLoopCarriedCounterIsNotOnePeeledIteration pins the loop-header merge. A
// term written in a loop body holds the join of its pre-loop value and every
// value the body carries back, widened to its representation, so a post-loop
// guard the counter can satisfy keeps both arms live. Analysing one peeled
// iteration instead would decide the guard from a trip count the loop never
// fixed and silently drop the arm behind it.
func TestLoopCarriedCounterIsNotOnePeeledIteration(t *testing.T) {
	reached := checkSource(t, `local total: integer = 0
for _ = 1, math.random(10) do
    total = total + 1
end
if total >= 2 then
    local mismatched: string = 1
    print(mismatched)
end
`)
	if !strings.Contains(diagnosticSummaries(reached), "cannot assign mismatched") {
		t.Fatalf("an arm a multi-trip counter reaches was not analysed:\n%s", diagnosticSummaries(reached))
	}
	// The representation is kept: integer addition is closed, so the widened
	// carrier still satisfies its integer declaration.
	representation := checkSource(t, `local total: integer = 0
for _ = 1, math.random(10) do
    total = total + 1
end
local kept: integer = total
print(kept)
`)
	if summaries := diagnosticSummaries(representation); summaries != "" {
		t.Fatalf("the widened counter lost its integer representation:\n%s", summaries)
	}
	// A term the body never writes is not loop-carried and keeps its exact
	// value, so its guard still decides.
	untouched := checkSource(t, `local total: integer = 0
for _ = 1, math.random(10) do
    print(total)
end
if total >= 2 then
    local unreachable: string = 1
    print(unreachable)
end
`)
	if summaries := diagnosticSummaries(untouched); summaries != "" {
		t.Fatalf("a term no trip writes was widened anyway:\n%s", summaries)
	}
}

// TestLoopBodyMemberWriteIsPossibleNotEstablished pins the loop-body member
// publication. Running no trip is one of a loop's executions, so a slot the
// body introduces is possible afterwards however unconditional the write is.
// A slot a pre-loop write already established keeps what the join of both
// admits, and a write past no back edge still establishes its slot exactly.
func TestLoopBodyMemberWriteIsPossibleNotEstablished(t *testing.T) {
	unconditional := checkSource(t, `local buf: {string} = {}
for _, s in ipairs({"a"}) do
    buf[1] = s
end
local first: string = buf[1]
print(first)
`)
	if !strings.Contains(diagnosticSummaries(unconditional), "buf[1]") {
		t.Fatalf("an unconditional loop-body write was published as an established member:\n%s", diagnosticSummaries(unconditional))
	}
	conditional := checkSource(t, `local buf: {string} = {}
for _, s in ipairs({"a"}) do
    if s ~= "" then
        buf[1] = s
    end
end
local first: string = buf[1]
print(first)
`)
	if !strings.Contains(diagnosticSummaries(conditional), "buf[1]") {
		t.Fatalf("a conditional loop-body write was published as an established member:\n%s", diagnosticSummaries(conditional))
	}
	seeded := checkSource(t, `local t: {name: string} = {name = "start"}
for _, s in ipairs({"a"}) do
    t.name = s
end
local kept: string = t.name
print(kept)
`)
	if summaries := diagnosticSummaries(seeded); summaries != "" {
		t.Fatalf("a slot a pre-loop write established became optional:\n%s", summaries)
	}
	straight := checkSource(t, `local buf: {string} = {}
buf[1] = "a"
local first: string = buf[1]
print(first)
`)
	if summaries := diagnosticSummaries(straight); summaries != "" {
		t.Fatalf("a straight-line write stopped establishing its slot:\n%s", summaries)
	}
}

// TestLoopBodyExactKeyStoreIsPossibleNotEstablished pins the same publication
// for the store whose key the body resolves rather than spells. The key one
// trip resolves is that trip's iterate, so the slot it names is established by
// that trip alone and the execution that runs no trip reaches the read with the
// slot the loop introduced still empty. A counter advanced after the append is
// one ahead of the slot the append filled, so the count witnesses neither the
// slot below it nor the one it names.
func TestLoopBodyExactKeyStoreIsPossibleNotEstablished(t *testing.T) {
	tripOne := checkSource(t, `local function collect(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        buf[n] = s
        n = n + 1
    end
    if n >= 1 then
        local first: string = buf[1]
        return first
    end
    return ""
end
return collect
`)
	if !strings.Contains(diagnosticSummaries(tripOne), "buf[1]") {
		t.Fatalf("a slot only one trip's iterate named was published as an established member:\n%s", diagnosticSummaries(tripOne))
	}
	firstSlot := checkSource(t, `local function collect(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        buf[n] = s
        n = n + 1
    end
    if n >= 1 then
        local first: string = buf[0]
        return first
    end
    return ""
end
return collect
`)
	if !strings.Contains(diagnosticSummaries(firstSlot), "buf[0]") {
		t.Fatalf("the slot the first trip fills was established for the execution that runs no trip:\n%s", diagnosticSummaries(firstSlot))
	}
	constant := checkSource(t, `local function collect(src: {string}): string
    local buf: {string} = {}
    local k: integer = 1
    for _, s in ipairs(src) do
        buf[k] = s
    end
    local first: string = buf[1]
    return first
end
return collect
`)
	if !strings.Contains(diagnosticSummaries(constant), "buf[1]") {
		t.Fatalf("a resolved constant key established its slot for a cycle that may run no trip:\n%s", diagnosticSummaries(constant))
	}
	seeded := checkSource(t, `local function collect(src: {string}, seed: string): string
    local buf: {string} = {}
    local k: integer = 1
    buf[k] = seed
    local n: integer = 1
    for _, s in ipairs(src) do
        buf[n] = s
        n = n + 1
    end
    local first: string = buf[1]
    return first
end
return collect
`)
	if summaries := diagnosticSummaries(seeded); summaries != "" {
		t.Fatalf("a slot a pre-cycle exact-key write established became optional:\n%s", summaries)
	}
	// The counter paired with the append is the occupancy proof: the increment
	// precedes the store, so the density relation still answers this read.
	paired := checkSource(t, `local function collect(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end
    if n >= 1 then
        local first: string = buf[1]
        return first
    end
    return ""
end
return collect
`)
	if summaries := diagnosticSummaries(paired); summaries != "" {
		t.Fatalf("the density relation stopped witnessing the slot the count reaches:\n%s", summaries)
	}
	straight := checkSource(t, `local function collect(seed: string): string
    local buf: {string} = {}
    local k: integer = 1
    buf[k] = seed
    local first: string = buf[1]
    return first
end
return collect
`)
	if summaries := diagnosticSummaries(straight); summaries != "" {
		t.Fatalf("a straight-line exact-key store stopped establishing its slot:\n%s", summaries)
	}
}

// TestWhileExitCarriesWhatTheTripsWrote pins the loop-exit read for the arm-
// guarded loop forms. A while body publishes under the condition's continuing
// arm and the statements past the loop stand on the leaving arm, so a read there
// that reported the arm it can see alone would report the value the loop
// received rather than the value it produced.
func TestWhileExitCarriesWhatTheTripsWrote(t *testing.T) {
	counter := checkSource(t, `local more = function(): boolean return true end
local xs: {string} = {"a"}
local i: integer = 1
while more() do
    i = i + 1
end
if i <= #xs then
    local first: string = xs[i]
    print(first)
end
`)
	if !strings.Contains(diagnosticSummaries(counter), "main.lua:8") {
		t.Fatalf("a guarded read decided the counter from the value the loop received:\n%s", diagnosticSummaries(counter))
	}
	// The same read on a counter no trip advances keeps its exact value: a term
	// the body never writes is not carried around the back edge.
	untouched := checkSource(t, `local more = function(): boolean return true end
local xs: {string} = {"a"}
local i: integer = 1
while more() do
    print(i)
end
if i <= #xs then
    local first: string = xs[i]
    print(first)
end
`)
	if summaries := diagnosticSummaries(untouched); summaries != "" {
		t.Fatalf("a counter no trip writes was widened by the loop anyway:\n%s", summaries)
	}
}

// TestWhileExitKeepsTheConditionItLeftOn is the precision guardrail. The exit
// arm's own narrowing is derived from the value entering the condition, which is
// already the join over every trip, so the exit keeps it instead of joining it
// back against the arm that continued.
func TestWhileExitKeepsTheConditionItLeftOn(t *testing.T) {
	narrowed := checkSource(t, `local next = function(): string? return nil end
local x: string? = next()
while x ~= nil do
    x = next()
end
local done: string = x
print(done)
`)
	if !strings.Contains(diagnosticSummaries(narrowed), "it is nil") {
		t.Fatalf("the loop exit lost the condition that ended the loop:\n%s", diagnosticSummaries(narrowed))
	}
}

// TestRefutedClaimBindsItsCellForTheJoin pins what a refuted assignment leaves
// behind. The assignment happens: the cell holds the value the source carried,
// not the type the claim named, so a later join over the arm that refuted and
// the arm that did not states the carrier both produce. Taking the claimed type
// instead would let an unproven claim decide the join in its own favour.
func TestRefutedClaimBindsItsCellForTheJoin(t *testing.T) {
	drifted := checkSource(t, `local function drift(more: () -> boolean, halve: () -> boolean): integer
    local i: integer = 7
    while more() do
        if halve() then
            i = i / 2
        else
            i = i + 1
        end
    end
    local j: integer = i
    return j
end
return drift
`)
	if !strings.Contains(diagnosticSummaries(drifted), "main.lua:10") {
		t.Fatalf("the arm that refuted its declaration contributed nothing to the join:\n%s", diagnosticSummaries(drifted))
	}
	if !strings.Contains(diagnosticSummaries(drifted), "it is number, not integer") {
		t.Fatalf("the join reported a carrier neither arm produces:\n%s", diagnosticSummaries(drifted))
	}
	// An arm whose assignment satisfies its declaration keeps deciding the join
	// exactly as before: nothing here depends on a claim having been refuted.
	kept := checkSource(t, `local function kept(more: () -> boolean, twice: () -> boolean): integer
    local i: integer = 7
    while more() do
        if twice() then
            i = i + 2
        else
            i = i + 1
        end
    end
    local j: integer = i
    return j
end
return kept
`)
	if summaries := diagnosticSummaries(kept); summaries != "" {
		t.Fatalf("a loop whose every arm keeps its declaration was refuted anyway:\n%s", summaries)
	}
}

// TestUnprovenClaimStaysUnprovenWhereItIsRead is the trust guardrail for the
// same publication. A cell whose current value is an unproven claim still reads
// as one: only a join, which is no single edge's statement, consumes the value
// the claim bound.
func TestUnprovenClaimStaysUnprovenWhereItIsRead(t *testing.T) {
	direct := checkSource(t, `local function direct(): string
    local i: integer = "text"
    local j: string = i
    return j
end
return direct
`)
	if !strings.Contains(diagnosticSummaries(direct), "cannot assign i") {
		t.Fatalf("a refuted declaration stopped reporting:\n%s", diagnosticSummaries(direct))
	}
	if strings.Contains(diagnosticSummaries(direct), "cannot assign j") {
		t.Fatalf("a read took the value an unproven claim bound as a proof:\n%s", diagnosticSummaries(direct))
	}
}
