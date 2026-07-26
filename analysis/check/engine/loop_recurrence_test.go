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
