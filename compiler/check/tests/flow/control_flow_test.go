package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestControlFlow_If(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "simple if",
			Code:      `if true then end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "if else",
			Code:      `if true then local x = 1 else local x = 2 end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "if elseif else",
			Code:      `if true then local x = 1 elseif false then local x = 2 else local x = 3 end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "if local scope isolation",
			Code: `
				if true then
					local x = 1
				end
				local y: number = x
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestControlFlow_DoBlock(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "do block creates scope",
			Code: `
				do
					local x = 1
				end
				local y: number = x
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "nested do blocks",
			Code: `
				local x = 0
				do
					local y = 1
					do
						local z = 2
						x = z
					end
				end
				local result: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestControlFlow_Return(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "return correct type",
			Code: `
				local function f(): number
					return 42
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "return wrong type",
			Code: `
				local function f(): number
					return "wrong"
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "early return in if",
			Code: `
				local function f(x: number): number
					if x < 0 then
						return 0
					end
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple return values",
			Code: `
				local function f(): (number, string)
					return 1, "ok"
				end
				local a: number, b: string = f()
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestControlFlow_Break(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "break in while",
			Code: `
				local i = 0
				while true do
					i = i + 1
					if i > 10 then break end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "break in for",
			Code: `
				for i = 1, 100 do
					if i > 10 then break end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestControlFlow_ErrorReturn(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "optional error return",
			Code: `
				local function div(a: number, b: number): (number?, string?)
					if b == 0 then
						return nil, "division by zero"
					end
					return a / b, nil
				end
				local result, err = div(10, 2)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestControlFlow_TypePreservation tests that types are preserved through
// callbacks and higher-order functions.
func TestControlFlow_TypePreservation(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "callback_preserves_type",
			Code: `
				type Handler = fun(data: string): nil
				local function process(items: {string}, handler: Handler)
					for _, item in ipairs(items) do
						handler(item)
					end
				end
				process({"a", "b"}, function(s: string)
					local upper: string = s:upper()
				end)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "higher_order_function_types",
			Code: `
				type Mapper<T, U> = fun(x: T): U
				local function map<T, U>(arr: {T}, f: Mapper<T, U>): {U}
					local result: {U} = {}
					for i, v in ipairs(arr) do
						result[i] = f(v)
					end
					return result
				end
				local nums = map({"1", "2", "3"}, function(s: string): number
					return tonumber(s) or 0
				end)
				local n: number = nums[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "closure_captures_type",
			Code: `
				local function make_adder(n: number): fun(x: number): number
					return function(x: number): number
						return x + n
					end
				end
				local add5 = make_adder(5)
				local result: number = add5(10)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "callback_result_preserves_type",
			Code: `
				local function apply<T, U>(value: T, fn: fun(x: T): U): U
					return fn(value)
				end
				local result: string = apply(42, function(n: number): string
					return tostring(n)
				end)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
