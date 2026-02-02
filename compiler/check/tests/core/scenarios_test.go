package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestCoreScenarios tests various core type checking scenarios.
func TestCoreScenarios(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "assign widening allows mixed types",
			Code: `
				local x = 1
				x = "ok"
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "method param self substitution",
			Code: `
				type T = { eq: (self: T, other: T) -> boolean }
				local t: T = { eq = function(self, other) return self == other end }
				local ok = t:eq(t)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "empty table allows map assignments",
			Code: `
				local t = {}
				t["a"] = 1
				t["b"] = true
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "untyped params allow missing args",
			Code: `
				local function eq(actual, expected, msg)
					return actual == expected
				end
				eq(1, 1)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "tonumber skips optional error return",
			Code: `
				type Request = { query: (self: Request, key: string) -> (string?, Error?) }
				local function handler(req: Request)
					local code = tonumber(req:query("code")) or 200
					return code
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "record literal assignable to record",
			Code: `
				local person: {name: string, age: number} = { name = "Alice", age = 30 }
				return person
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "record literal assignable to intersection",
			Code: `
				type Person = {name: string} & {age: number}
				local p: Person = { name = "Alice", age = 30 }
				return p
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "attr call does not recurse",
			Code: `
				local t = { print = function(msg) return msg end }
				t.print("hi")
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestControlFlow tests control flow statements.
func TestControlFlow(t *testing.T) {
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
			Name:      "while loop",
			Code:      `while true do break end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "for loop",
			Code:      `for i = 1, 10 do end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "for loop with step",
			Code:      `for i = 1, 10, 2 do end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "for loop string init",
			Code:      `for i = "a", 10 do end`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name:      "generic for with pairs",
			Code:      `for k, v in pairs({}) do end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "generic for with ipairs",
			Code:      `for i, v in ipairs({}) do end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "repeat until",
			Code:      `repeat local x = 1 until true`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "do block",
			Code:      `do local x = 1 end`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestExpressions tests expression evaluation.
func TestExpressions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "arithmetic",
			Code:      `local x = 1 + 2 * 3`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "string concat",
			Code:      `local s = "hello" .. " world"`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "comparison",
			Code:      `local b = 1 < 2`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "logical and",
			Code:      `local x = true and false`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "logical or",
			Code:      `local x = nil or 1`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "unary not",
			Code:      `local b = not true`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "unary minus",
			Code:      `local x = -42`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "length operator",
			Code:      `local n = #"hello"`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "concat uses first return value",
			Code: `
				local function f()
					return "a", 1
				end
				local s = f() .. "b"
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "length uses first return value",
			Code: `
				local function f()
					return {1, 2, 3}, 10
				end
				local n = #f()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "function call",
			Code:      `print("hello")`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "method call on literal",
			Code:      `local s = ("hello"):upper()`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "table access",
			Code:      `local t = {x = 1}; local v = t.x`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "array index",
			Code:      `local a = {1, 2, 3}; local v = a[1]`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTypeAnnotations tests type annotation scenarios.
func TestTypeAnnotations(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "optional type",
			Code:      `local x: number? = nil`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "union type",
			Code:      `local x: number | string = 1`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "array type inferred",
			Code:      `local arr = {1, 2, 3}`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "record type inferred",
			Code:      `local r = {x = 1, y = "a"}`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "function type declared",
			Code:      `local f: (number, string) -> boolean = function(a: number, b: string): boolean return true end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "array type mismatch",
			Code:      `local arr: {string} = {1, 2, 3}`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestStdlibUsage tests stdlib function usage.
func TestStdlibUsage(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "print",
			Code:      `print("hello")`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "type function",
			Code:      `local t = type(42)`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "pairs",
			Code:      `for k, v in pairs({a = 1}) do end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "ipairs",
			Code:      `for i, v in ipairs({1, 2, 3}) do end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "math.floor",
			Code:      `local x = math.floor(3.5)`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "string.upper",
			Code:      `local s = string.upper("hello")`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "table.insert",
			Code:      `local t = {}; table.insert(t, 1)`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "tostring",
			Code:      `local s = tostring(42)`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "tonumber",
			Code:      `local n = tonumber("42")`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestBasicNarrowing tests basic narrowing scenarios.
func TestBasicNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "nil check then branch",
			Code: `
				function f(x: string?)
					if x ~= nil then
						local s: string = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nil check else branch inline",
			Code: `
				function f(x: string?)
					if x == nil then
						return
					else
						local s: string = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "truthy check",
			Code: `
				function f(x: string?)
					if x then
						local s: string = x
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "no narrowing without check",
			Code: `
				function f(x: string?)
					local s: string = x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "nested narrowing",
			Code: `
				function f(x: string?, y: number?)
					if x ~= nil then
						if y ~= nil then
							local s: string = x
							local n: number = y
						end
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTypeIsNarrowing tests Type:is narrowing.
func TestTypeIsNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "Type:is basic pattern",
			Code: `
				type Point = {x: number, y: number}
				function validate(data: any)
					local p = Point:is(data)
					if p then
						local p: {x: number, y: number} = data
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type:is truthy check",
			Code: `
				type Point = {x: number, y: number}
				local function isPoint(x)
					return Point:is(x)
				end
				function validate(data: any)
					local val = isPoint(data)
					if val then
						local p: {x: number, y: number} = data
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type:is wrapper falsy excludes",
			Code: `
				type Point = {x: number, y: number}
				local function isPoint(x)
					return Point:is(x)
				end
				function validate(data: any)
					local p = isPoint(data)
					if not p then
						local p: {x: number, y: number} = data
					end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "Type:is direct condition narrows",
			Code: `
				type Point = {x: number, y: number}
				function validate(data: any)
					if Point:is(data) then
						local p: {x: number, y: number} = data
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestComplexScenarios tests complex code scenarios.
func TestComplexScenarios(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "recursive function",
			Code: `
				function factorial(n: number): number
					if n <= 1 then
						return 1
					else
						return n * factorial(n - 1)
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested functions",
			Code: `
				function outer(x: number): number
					local function inner(y: number): number
						return y * 2
					end
					return inner(x) + 1
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "closure",
			Code: `
				function counter(): () -> number
					local count = 0
					return function(): number
						count = count + 1
						return count
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "table with methods",
			Code: `
				local obj = {
					value = 0,
					get = function(self): number
						return self.value
					end
				}
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "early return guard",
			Code: `
				function process(x: number?): number
					if x == nil then
						return 0
					end
					return x * 2
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestModulePatterns tests module definition patterns.
func TestModulePatterns(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "local module definition",
			Code: `
				local M = {}
				function M.add(a: number, b: number): number
					return a + b
				end
				function M.sub(a: number, b: number): number
					return a - b
				end
				local result: number = M.add(1, 2)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "module with method",
			Code: `
				local Counter = {count = 0}
				function Counter:increment()
					self.count = self.count + 1
				end
				function Counter:get(): number
					return self.count
				end
				Counter:increment()
				local n: number = Counter:get()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "module return pattern",
			Code: `
				local M = {}
				M.version = "1.0"
				function M.init()
					print("initialized")
				end
				return M
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestUntypedStringOps tests untyped parameter string operations.
func TestUntypedStringOps(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "concat and length on untyped params",
			Code: `
				local function green(s) return "\027[32m" .. s .. "\027[0m" end
				local function greet(name)
					if name and #name > 0 then
						return "Hello, " .. name
					end
					return green("stranger")
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
