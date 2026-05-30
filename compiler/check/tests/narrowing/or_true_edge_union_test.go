package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestOrTrueEdgeUnionNarrowing covers the `A or B` TRUE-edge union narrowing.
// A bare `if A or B` is lowered to two short-circuit branches whose true edges
// merge at the body (the merge-LUB rebuilds the union), but a nested `(A or B)`
// (an operand of an outer logical op) is kept as a single branch whose Condition
// is a *ast.LogicalOpExpr. Its true edge proves only the existential "at least one
// holds"; when both operands narrow the SAME value it pins the value to the UNION
// of each operand's narrowing, and when they narrow different values it narrows
// nothing.
func TestOrTrueEdgeUnionNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			// On the true edge of `(r.kind=="a" or r.kind=="b")`, r is A|B; both have
			// run, so the call type-checks. Without union narrowing r stays A|B|C and
			// C lacks run.
			Name: "or_same_value_narrows_to_union",
			Code: `
				type A = {kind: "a", run: fun(): string}
				type B = {kind: "b", run: fun(): string}
				type C = {kind: "c"}
				local function get(): A | B | C
					return {kind = "a", run = function(): string return "a" end}
				end
				local r: A | B | C = get()
				local flag = true
				if (r.kind == "a" or r.kind == "b") and flag then
					local s: string = r.run()
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			// SANITY: with no guard the call over the full A|B|C union must fail (C
			// lacks run), proving the call is type-checked.
			Name: "no_guard_full_union_method_fails",
			Code: `
				type A = {kind: "a", run: fun(): string}
				type B = {kind: "b", run: fun(): string}
				type C = {kind: "c"}
				local function get(): A | B | C
					return {kind = "a", run = function(): string return "a" end}
				end
				local r: A | B | C = get()
				local s: string = r.run()
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			// NEGATIVE: the union edge is A|B, NOT A alone; an A-only field access must
			// still fail (no over-narrow to a single variant).
			Name: "or_union_is_not_single_variant",
			Code: `
				type A = {kind: "a", a_field: number}
				type B = {kind: "b", b_field: string}
				type C = {kind: "c"}
				local function get(): A | B | C
					return {kind = "a", a_field = 1}
				end
				local r: A | B | C = get()
				local flag = true
				if (r.kind == "a" or r.kind == "b") and flag then
					local n: number = r.a_field
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			// NEGATIVE: operands narrow DIFFERENT values, so the true edge proves only
			// an existential and narrows nothing; r stays A|B|C and r.run fails.
			Name: "or_different_values_no_narrow",
			Code: `
				type A = {kind: "a", run: fun(): string}
				type B = {kind: "b", run: fun(): string}
				type C = {kind: "c"}
				local function get(): A | B | C
					return {kind = "a", run = function(): string return "a" end}
				end
				local r: A | B | C = get()
				local q: A | B | C = get()
				local flag = true
				if (r.kind == "a" or q.kind == "b") and flag then
					local s: string = r.run()
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			// type() checks on the same value also union-narrow: on the true edge x is
			// string|number (the scalar union), assignable to a string|number slot.
			Name: "or_typeof_same_value_narrows_to_union",
			Code: `
				local function get(): string | number | boolean
					return "hi"
				end
				local x: string | number | boolean = get()
				local flag = true
				if (type(x) == "string" or type(x) == "number") and flag then
					local y: string | number = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
