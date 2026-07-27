package front_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// monotoneCarrierCount reports how many carriers the branch decisions of the
// named nested body carry. The operand is the body's whole statement about its
// counters, so its presence or absence is the proof under test.
func monotoneCarrierCount(t *testing.T, source string) int {
	t.Helper()
	compilation, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	carriers := 0
	var walk func(front.Compilation)
	walk = func(item front.Compilation) {
		for _, operation := range item.Artifact.Equations {
			if operation.Occurrence.Kind != "branch-relations" {
				continue
			}
			for _, operand := range operation.Operands {
				if operand.Role.InFamily(equation.RoleFamilyMonotoneFloor) {
					carriers++
				}
			}
		}
		for _, nested := range item.Nested {
			walk(nested)
		}
	}
	walk(compilation)
	return carriers
}

// TestMonotoneCarrierAcceptsOnlyNonNegativeConstantSteps pins the arithmetic
// fragment the proof is closed over. Only a carrier plus a non-negative
// constant keeps the seed; every other write to the same path is a value this
// body cannot bound from below, and one of them withdraws the carrier.
func TestMonotoneCarrierAcceptsOnlyNonNegativeConstantSteps(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		body   string
		wanted bool
	}{
		{name: "increment", body: "i = i + 1", wanted: true},
		{name: "zero step", body: "i = i + 0", wanted: true},
		{name: "constant on the left", body: "i = 1 + i", wanted: true},
		{name: "decrement", body: "i = i - 1"},
		{name: "negative constant", body: "i = i + -1"},
		{name: "variable step", body: "i = i + step"},
		{name: "product", body: "i = i * 2"},
		{name: "quotient", body: "i = i / 2"},
		{name: "unrelated source", body: "i = step"},
	} {
		source := `local xs: {number} = {}
local step: number = 2
local i: number = 1
while i <= #xs do
    ` + testCase.body + `
end
`
		carriers := monotoneCarrierCount(t, source)
		if (carriers != 0) != testCase.wanted {
			t.Fatalf("%s: carriers=%d, want proven=%v", testCase.name, carriers, testCase.wanted)
		}
	}
}

// TestMonotoneCarrierRequiresASeedOfAtLeastOne pins the other half of the write
// set. The bound the operand states is `>= 1`, so a seed below it, or a second
// assignment outside the cycle that is below it, states nothing.
func TestMonotoneCarrierRequiresASeedOfAtLeastOne(t *testing.T) {
	proven := `local xs: {number} = {}
local i: number = 1
while i <= #xs do
    i = i + 1
end
`
	if monotoneCarrierCount(t, proven) == 0 {
		t.Fatal("a counter seeded at one carries no floor")
	}
	fromZero := `local xs: {number} = {}
local i: number = 0
while i <= #xs do
    i = i + 1
end
`
	if carriers := monotoneCarrierCount(t, fromZero); carriers != 0 {
		t.Fatalf("a counter seeded at zero carried %d floors", carriers)
	}
	reseeded := `local xs: {number} = {}
local low: boolean = true
local i: number = 1
if low then
    i = 0
end
while i <= #xs do
    i = i + 1
end
`
	if carriers := monotoneCarrierCount(t, reseeded); carriers != 0 {
		t.Fatalf("a counter reseeded below one carried %d floors", carriers)
	}
}

// TestMonotoneCarrierWithdrawsACapturedCounter pins the completeness the proof
// rests on. A closure allocation hands its captures a cell this body no longer
// owns, so the callee may write the counter at any later point and this body's
// write set no longer accounts for every write to it.
func TestMonotoneCarrierWithdrawsACapturedCounter(t *testing.T) {
	captured := `local xs: {number} = {}
local i: number = 1
local reset = function() i = 0 end
while i <= #xs do
    i = i + 1
    reset()
end
`
	if carriers := monotoneCarrierCount(t, captured); carriers != 0 {
		t.Fatalf("a captured counter carried %d floors", carriers)
	}
}

// TestMonotoneCarrierIgnoresRefinementsOfItself pins that an annotation on the
// counter is not a write that withdraws it: a claim narrows the value already
// bound to a subset of itself and can produce nothing below it.
func TestMonotoneCarrierIgnoresRefinementsOfItself(t *testing.T) {
	annotated := `local xs: {number} = {}
local i: number = 1
while i <= #xs do
    local j: number = i
    i = i + 1
end
`
	if monotoneCarrierCount(t, annotated) == 0 {
		t.Fatal("an annotated counter carries no floor")
	}
}

// TestAcyclicBodyCarriesNoMonotoneOperand pins the topology the operand belongs
// to. A body with no cycle has no back edge for a carrier to survive, so it
// states nothing and its lowering is the one it already had.
func TestAcyclicBodyCarriesNoMonotoneOperand(t *testing.T) {
	acyclic := `local xs: {number} = {}
local i: number = 1
if i <= #xs then
    i = i + 1
end
`
	if carriers := monotoneCarrierCount(t, acyclic); carriers != 0 {
		t.Fatalf("an acyclic body carried %d monotone operands", carriers)
	}
}
