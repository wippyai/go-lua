package engine_test

import (
	"strings"
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

func TestCheckRoutesWhileThroughFrozenCyclicVM(t *testing.T) {
	result, err := engine.Check(`
local total = 0
while false do
    total = 1
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["total"]; got != "0" {
		t.Fatalf("published total = %q, want 0; values = %#v", got, result.Values)
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

func TestCheckBinaryComparisons(t *testing.T) {
	for name, source := range map[string]string{
		"equal numbers":    `local result = 2 == 2`,
		"unequal strings":  `local result = "left" ~= "right"`,
		"numeric ordering": `local result = 2 < 3`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := engine.Check(source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got := valuesByName(result.Values)["result"]; got != "true" {
				t.Fatalf("comparison = %q, want true; values = %#v", got, result.Values)
			}
		})
	}
}

func TestCheckUnknownBranchSelectsNoArm(t *testing.T) {
	for name, source := range map[string]string{
		"truthiness":       `local input = provider(); if input then local selected = true end`,
		"numeric relation": `local input = provider(); if input >= 1 then local selected = true end`,
		"index relation":   `local input = provider(); if input[1] then local selected = true end`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := engine.Check(source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if _, selected := valuesByName(result.Values)["selected"]; selected {
				t.Fatalf("unknown selector chose an arm: %#v", result.Values)
			}
		})
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
	if result.Transactions != 10 { // entry plus constructor topology, result, and closed-entry writes
		t.Fatalf("transactions = %d, want complete allocation writes", result.Transactions)
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
	if result.Transactions != 5 { // entry, apply, external boundary, call-results, assignment
		t.Fatalf("transactions = %d, want entry plus complete provider call sequence and assignment", result.Transactions)
	}
}

func TestCheckDynamicIndexReadsPublishConservativeUnknown(t *testing.T) {
	for name, source := range map[string]string{
		"path destination":      `local key = "missing"; local result = record[key]; local observed = result`,
		"temporary destination": `local key = "missing"; local result = record[key].field; local observed = result`,
		"nested dynamic key":    `local first = "one"; local second = "two"; local result = record[first][second]; local observed = result`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := engine.Check(source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got := valuesByName(result.Values)["observed"]; got != "unknown" {
				t.Fatalf("dynamic result = %q, want conservative unknown; values = %#v", got, result.Values)
			}
		})
	}
}

func TestCheckPublishesOrderedReturnTuple(t *testing.T) {
	result, err := engine.Check(`
local answer = 42
return answer, nil, false
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Outcomes)
	for key, want := range map[string]string{
		"return/arity": "3",
		"return/0":     "42",
		"return/1":     "nil",
		"return/2":     "false",
	} {
		if got[key] != want {
			t.Errorf("published %s = %q, want %q; outcomes = %#v", key, got[key], want, result.Outcomes)
		}
	}
}

func TestCheckPublishesEmptyReturnTuple(t *testing.T) {
	result, err := engine.Check("return")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Outcomes)
	if got["return/arity"] != "0" {
		t.Fatalf("published return arity = %q, want 0; outcomes = %#v", got["return/arity"], result.Outcomes)
	}
	if _, found := got["return/0"]; found {
		t.Fatalf("empty return published a first value: %#v", result.Outcomes)
	}
}

func TestCheckRetainsDistinctUnprovenClaimDiagnostics(t *testing.T) {
	result, err := engine.Check(`
local text: string = 1
local count: number = "one"
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Diagnostics)
	if len(got) != 2 {
		t.Fatalf("claim diagnostics = %#v, want two distinct facts", result.Diagnostics)
	}
	for _, want := range []string{`claim "number" is not proven`, `claim "string" is not proven`} {
		found := false
		for key, detail := range got {
			if strings.HasPrefix(key, "claim/unproven/op-") && detail == want {
				found = true
			}
		}
		if !found {
			t.Errorf("claim diagnostics = %#v, missing %q", result.Diagnostics, want)
		}
	}
}

func TestCheckUsesTopForUnmaterializedMemberRead(t *testing.T) {
	result, err := engine.Check(`
local record = provider()
local name = record.name
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["name"]; got != "unknown" {
		t.Fatalf("member read = %q, want unknown; values = %#v", got, result.Values)
	}
}

func TestCheckUnknownClaimDoesNotChooseABranch(t *testing.T) {
	result, err := engine.Check(`
local raw = provider()
local value = raw :: string
local result
if value then
    result = "then"
else
    result = "else"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["result"]; got != "nil" {
		t.Fatalf("unknown claim selected a branch: result=%q values=%#v", got, result.Values)
	}
}

func TestCheckUsesTopForUnmaterializedCurrentMemberRead(t *testing.T) {
	result, err := engine.Check(`
local record = provider()
local count = record.count + 1
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["count"]; got != "unknown" {
		t.Fatalf("member arithmetic = %q, want unknown; values = %#v", got, result.Values)
	}
}

func TestCheckPublishesAdjustedOpenReturnTailSlots(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   map[string]string
	}{
		{name: "open tail", source: `return provider()`, want: map[string]string{"return/arity": "1", "return/0": "unknown"}},
		{name: "prefix and open tail", source: `return "prefix", provider()`, want: map[string]string{"return/arity": "2", "return/0": `"prefix"`, "return/1": "unknown"}},
		{name: "parenthesized tail is adjusted", source: `return (provider())`, want: map[string]string{"return/arity": "1", "return/0": "unknown"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := engine.Check(test.source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			got := valuesByName(result.Outcomes)
			for key, want := range test.want {
				if got[key] != want {
					t.Errorf("published %s = %q, want %q; outcomes = %#v", key, got[key], want, result.Outcomes)
				}
			}
			if len(got) != len(test.want) {
				t.Errorf("published outcomes = %#v, want exactly %#v", got, test.want)
			}
		})
	}
}

// This is the slot-retention regression: the false-arm transaction contributes
// nothing, while the selected arm's same-named slot must survive VM closure
// merging intact.
func TestCheckGuardedReturnRetainsSelectedSlotAtMerge(t *testing.T) {
	result, err := engine.Check(`
if false then
    return "then"
else
    return "else"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Outcomes)
	if got["return/arity"] != "1" || got["return/0"] != `"else"` {
		t.Fatalf("guarded return outcomes = %#v, want the selected else slot", result.Outcomes)
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

func TestCheckWholeModuleShapedFile(t *testing.T) {
	result, err := engine.Check(`
local dependency = require("dependency")

local function first(value)
    return value
end

local function second()
    local local_value = first(42)
    return local_value
end

local answer = second()
return answer
`)
	if err != nil {
		t.Fatalf("Check whole file: %v", err)
	}
	if len(result.Artifact.Equations) == 0 {
		t.Fatal("Check whole file returned an empty artifact")
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
