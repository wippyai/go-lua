package engine

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// budgetProbeSource is an ordinary program with enough evaluated structure to
// spend a non-trivial amount of the work budget: a declared record, methods
// reached through a table, nested closures over a captured accumulator, loops,
// branches, and returned values the caller reads back.
const budgetProbeSource = `
type Entry = { id: string, weight: number }

local function classify(value: number): string
	if value > 20 then
		return "high"
	elseif value > 10 then
		return "medium"
	end
	return "low"
end

local registry = {}
registry.entries = {}

function registry.add(entry: Entry)
	registry.entries[entry.id] = entry
end

function registry.weigh(entry: Entry): number
	local scale = 1
	for step = 1, 4 do
		scale = scale + step
	end
	return entry.weight * scale
end

local function accumulate(seed: number)
	local total = seed
	local labels = {}
	return function(entry: Entry): string
		total = total + registry.weigh(entry)
		local label = classify(total)
		labels[entry.id] = label
		return label
	end, function(): number
		return total
	end
end

local step, current = accumulate(0)
local first = step({ id = "a", weight = 2 })
local second = step({ id = "b", weight = 7 })
registry.add({ id = "c", weight = 3 })

local report = { first = first, second = second, total = current() }
if report.total > 0 then
	report.first = classify(report.total)
end
return report
`

func factsEqual(left, right []equation.Fact) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Key != right[index].Key || !bytes.Equal(left[index].Value, right[index].Value) {
			return false
		}
		if len(left[index].Guards) != len(right[index].Guards) {
			return false
		}
	}
	return true
}

func publishedEqual(left, right []PublishedDiagnostic) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Code != right[index].Code || left[index].Message != right[index].Message || left[index].Span != right[index].Span {
			return false
		}
		if left[index].Fact.Key != right[index].Fact.Key || !bytes.Equal(left[index].Fact.Value, right[index].Fact.Value) {
			return false
		}
	}
	return true
}

func resultsEqual(left, right Result) bool {
	return factsEqual(left.Values, right.Values) &&
		factsEqual(left.Outcomes, right.Outcomes) &&
		factsEqual(left.Diagnostics, right.Diagnostics) &&
		factsEqual(left.ReturnCandidates, right.ReturnCandidates) &&
		factsEqual(left.ValueFacts, right.ValueFacts) &&
		publishedEqual(left.PublishedDiagnostics, right.PublishedDiagnostics) &&
		publishedEqual(left.PolicyDiagnostics, right.PolicyDiagnostics) &&
		left.Transactions == right.Transactions
}

// TestWorkBudgetSpendIsAFunctionOfTheInput is the determinism claim itself.
// The verdict and the work it took to reach it must be reproduced exactly by a
// second analysis of the same source, because nothing the evaluation counts is
// read from the host.
func TestWorkBudgetSpendIsAFunctionOfTheInput(t *testing.T) {
	first, firstBudget, err := checkWithWorkBudget(budgetProbeSource, nil, nil, nil, nil, defaultWorkBudget)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	if firstBudget == nil {
		t.Fatal("first check reported no work budget")
	}
	if firstBudget.spent == 0 {
		t.Fatal("first check spent no budget; the probe source evaluates nothing")
	}
	if firstBudget.overspent() {
		t.Fatalf("probe source exhausted the default budget after %d units", firstBudget.spent)
	}
	for attempt := 0; attempt < 4; attempt++ {
		next, nextBudget, err := checkWithWorkBudget(budgetProbeSource, nil, nil, nil, nil, defaultWorkBudget)
		if err != nil {
			t.Fatalf("repeat check %d: %v", attempt, err)
		}
		if nextBudget.spent != firstBudget.spent {
			t.Fatalf("repeat check %d spent %d units, first spent %d: evaluation work is not a function of the input", attempt, nextBudget.spent, firstBudget.spent)
		}
		if !resultsEqual(first, next) {
			t.Fatalf("repeat check %d published a different result than the first", attempt)
		}
	}
}

// TestWorkBudgetAboveDemandDoesNotChangeTheVerdict pins the budget as a bound
// rather than an input to the analysis: every budget at or above what the
// source demands must publish the identical result, so raising the limit can
// never buy a different answer for a program that already fits.
func TestWorkBudgetAboveDemandDoesNotChangeTheVerdict(t *testing.T) {
	reference, budget, err := checkWithWorkBudget(budgetProbeSource, nil, nil, nil, nil, defaultWorkBudget)
	if err != nil {
		t.Fatalf("reference check: %v", err)
	}
	demand := budget.spent
	for _, limit := range []uint64{demand, demand + 1, demand * 2, defaultWorkBudget} {
		result, used, err := checkWithWorkBudget(budgetProbeSource, nil, nil, nil, nil, limit)
		if err != nil {
			t.Fatalf("check at limit %d: %v", limit, err)
		}
		if used.overspent() {
			t.Fatalf("limit %d exhausted a budget that demand %d fits within", limit, demand)
		}
		if !resultsEqual(reference, result) {
			t.Fatalf("limit %d published a different result than the reference budget", limit)
		}
	}
}

// TestWorkBudgetExhaustionPublishesOnlyTheConservativeDiagnostic proves the
// degradation is fail-closed: an evaluation cut short by its budget publishes
// the conservative diagnostic alone, never a partial value or outcome surface
// that a consumer could mistake for a completed analysis.
func TestWorkBudgetExhaustionPublishesOnlyTheConservativeDiagnostic(t *testing.T) {
	result, budget, err := checkWithWorkBudget(budgetProbeSource, nil, nil, nil, nil, 1)
	if err != nil {
		t.Fatalf("starved check returned a hard error: %v", err)
	}
	if !budget.overspent() {
		t.Fatal("a one-unit budget was not exhausted by the probe source")
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("starved check published %d diagnostics, want exactly the conservative one", len(result.Diagnostics))
	}
	if result.Diagnostics[0].Key != "analysis/conservative" {
		t.Fatalf("starved check published %q, want analysis/conservative", result.Diagnostics[0].Key)
	}
	if !strings.Contains(string(result.Diagnostics[0].Value), "work budget") {
		t.Fatalf("conservative diagnostic does not name the exhausted budget: %q", result.Diagnostics[0].Value)
	}
	if len(result.Values) != 0 || len(result.Outcomes) != 0 || len(result.PublishedDiagnostics) != 0 || result.Placement != nil {
		t.Fatalf("starved check published %d values, %d outcomes and %d source diagnostics beside the conservative bail", len(result.Values), len(result.Outcomes), len(result.PublishedDiagnostics))
	}
}

// TestWorkBudgetVerdictFlipsAtOneReproducibleLimit is the property a wall-clock
// deadline cannot have. The budget at which the source stops being analysed in
// full is a single value determined by the source, identical on every run, so
// two machines cannot disagree about the verdict.
func TestWorkBudgetVerdictFlipsAtOneReproducibleLimit(t *testing.T) {
	_, budget, err := checkWithWorkBudget(budgetProbeSource, nil, nil, nil, nil, defaultWorkBudget)
	if err != nil {
		t.Fatalf("reference check: %v", err)
	}
	demand := budget.spent
	if demand < 2 {
		t.Fatalf("probe source demands only %d units; it cannot exercise a budget threshold", demand)
	}
	for attempt := 0; attempt < 3; attempt++ {
		below, belowBudget, err := checkWithWorkBudget(budgetProbeSource, nil, nil, nil, nil, demand-1)
		if err != nil {
			t.Fatalf("below-threshold check %d: %v", attempt, err)
		}
		if !belowBudget.overspent() {
			t.Fatalf("below-threshold check %d did not exhaust a budget one unit under its demand of %d", attempt, demand)
		}
		if len(below.Diagnostics) != 1 || below.Diagnostics[0].Key != "analysis/conservative" {
			t.Fatalf("below-threshold check %d did not degrade to the conservative diagnostic", attempt)
		}
		_, aboveBudget, err := checkWithWorkBudget(budgetProbeSource, nil, nil, nil, nil, demand)
		if err != nil {
			t.Fatalf("at-threshold check %d: %v", attempt, err)
		}
		if aboveBudget.overspent() {
			t.Fatalf("at-threshold check %d exhausted a budget equal to its demand of %d", attempt, demand)
		}
		if aboveBudget.spent != demand {
			t.Fatalf("at-threshold check %d spent %d units, want the reproduced demand %d", attempt, aboveBudget.spent, demand)
		}
	}
}

// TestWorkBudgetStaysExhausted keeps the bail fail-closed across a caller that
// discards the error. The file boundary reads the budget, not only the return
// value, so a swallowed exhaustion still forces the conservative publication.
func TestWorkBudgetStaysExhausted(t *testing.T) {
	budget := newWorkBudget(4)
	if err := budget.charge(3); err != nil {
		t.Fatalf("charging within the limit failed: %v", err)
	}
	if budget.overspent() {
		t.Fatal("budget reported exhaustion while units remained")
	}
	if err := budget.charge(2); err == nil {
		t.Fatal("charging past the limit reported success")
	}
	if !budget.overspent() {
		t.Fatal("budget did not record its exhaustion")
	}
	if err := budget.charge(0); err == nil {
		t.Fatal("a zero-unit charge after exhaustion reported success")
	}
	if budget.err() == nil {
		t.Fatal("an exhausted budget reports no error at the file boundary")
	}
}
