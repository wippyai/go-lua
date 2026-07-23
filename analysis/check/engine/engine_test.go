package engine_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestCheckPublishesScalarAssignment(t *testing.T) {
	result, err := engine.Check(`local answer = 42`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["answer"]; got != "42" {
		t.Fatalf("published answer = %q, want 42; values = %#v", got, result.Values)
	}
	if result.Transactions != 2 {
		t.Fatalf("transactions = %d, want entry plus assignment", result.Transactions)
	}
}

func TestCheckTriviallyTrueBranchPublishesTruthinessNarrowing(t *testing.T) {
	result, err := engine.Check(`
local value = 1
if true then
    local narrowed = value
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["narrowed"]; got != "1" {
		t.Fatalf("published narrowed value = %q, want 1; values = %#v", got, result.Values)
	}
	if !hasFact(result.Outcomes, "narrowing/", "truthy") {
		t.Fatalf("outcomes did not contain a truthy narrowing: %#v", result.Outcomes)
	}
}

func TestCheckLiteralBranchSelectsOnlyMatchingArm(t *testing.T) {
	result, err := engine.Check(`
local status = "ready"
local selected
if status == "ready" then
    selected = "then"
else
    selected = "else"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["selected"]; got != `"then"` {
		t.Fatalf("published selected = %q, want Lua string spelling; values = %#v", got, result.Values)
	}
}

func TestCheckPathAndNilPredicates(t *testing.T) {
	result, err := engine.Check(`
local left = 3
local right = 3
local absent
local selected
if left == right then
    selected = "path"
end
if absent == nil then
    selected = "nil"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["selected"]; got != `"nil"` {
		t.Fatalf("published selected = %q, want Lua string spelling; values = %#v", got, result.Values)
	}
}

func TestCheckNumericBranchPredicate(t *testing.T) {
	result, err := engine.Check(`
local count = 3
local selected
if count >= 3 then
    selected = "then"
else
    selected = "else"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["selected"]; got != `"then"` {
		t.Fatalf("published selected = %q, want Lua string spelling; values = %#v", got, result.Values)
	}
}

func TestCheckDoesNotTurnAnAbsentPathIntoFalse(t *testing.T) {
	_, err := engine.Check(`
if not_bound_here then
    local selected = true
end
`)
	if err == nil {
		t.Fatal("Check accepted an absent branch path as a falsy value")
	}
}

func TestCheckEvaluatesClosedAllocationPairs(t *testing.T) {
	result, err := engine.Check(`
local object = { first = 1, child = { second = 2 } }
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Transactions != 5 { // entry plus template/materialization for each table
		t.Fatalf("transactions = %d, want complete allocation pairs", result.Transactions)
	}
}

func TestCheckUnknownCallPublishesExplicitUnknownResult(t *testing.T) {
	result, err := engine.Check(`local value = provider()`) // provider has no local outcome.
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["value"]; got != "unknown" {
		t.Fatalf("published value = %q, want explicit unknown; values = %#v", got, result.Values)
	}
	if result.Transactions != 4 { // entry, apply, call-results, assignment
		t.Fatalf("transactions = %d, want entry plus complete call sequence and assignment", result.Transactions)
	}
}

func TestCheckUnknownCallConditionDoesNotAuthorizeEitherGuardedArm(t *testing.T) {
	result, err := engine.Check(`
if provider() then
    local value = 1
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Values) != 0 || len(result.Outcomes) != 0 {
		t.Fatalf("unknown condition published guarded facts: values=%#v outcomes=%#v", result.Values, result.Outcomes)
	}
}

func valuesByName(values []equation.Fact) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.Key] = string(value.Value)
	}
	return result
}

func hasFact(facts []equation.Fact, prefix, value string) bool {
	for _, fact := range facts {
		if len(fact.Key) >= len(prefix) && fact.Key[:len(prefix)] == prefix && string(fact.Value) == value {
			return true
		}
	}
	return false
}
