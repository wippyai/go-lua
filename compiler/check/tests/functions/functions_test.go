package functions

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestFunction_Definition(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "simple function",
			Code:      `function f() end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "function with parameters",
			Code:      `function f(x: number, y: string) end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "function with return type",
			Code:      `function f(): number return 1 end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "wrong return type",
			Code:      `function f(): number return "hello" end`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name:      "multiple returns",
			Code:      `function f(): (number, string) return 1, "a" end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "variadic function",
			Code:      `function f(...: number) end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "local function",
			Code:      `local function f() end`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestFunction_Call(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "call with correct types",
			Code: `
				local function add(a: number, b: number): number
					return a + b
				end
				local x: number = add(1, 2)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "call with wrong argument type",
			Code: `
				local function add(a: number, b: number): number
					return a + b
				end
				local x = add(1, "wrong")
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "call result assigned to wrong type",
			Code: `
				local function add(a: number, b: number): number
					return a + b
				end
				local x: string = add(1, 2)
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "too few arguments",
			Code: `
				local function add(a: number, b: number): number
					return a + b
				end
				local x = add(1)
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "call non-callable",
			Code: `
				local x: number = 42
				local y = x()
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "variadic function correct",
			Code: `
				local function sum(...: number): number
					return 0
				end
				local x: number = sum(1, 2, 3, 4, 5)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "variadic function wrong type",
			Code: `
				local function sum(...: number): number
					return 0
				end
				local x = sum(1, 2, "three")
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "call statement correct",
			Code: `
				local function log(msg: string) end
				log("hello")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "call statement wrong type",
			Code: `
				local function log(msg: string) end
				log(123)
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "return call wrong argument type",
			Code: `
				local function add(a: number, b: number): number
					return a + b
				end
				local function f(): number
					return add("bad", 2)
				end
				local x = f()
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestFunction_Method(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "method call with self",
			Code: `
				type Counter = {
					count: number,
					increment: (self: Counter) -> number
				}
				local c: Counter = {
					count = 0,
					increment = function(self: Counter): number
						self.count = self.count + 1
						return self.count
					end
				}
				local n: number = c:increment()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "method call with parameters",
			Code: `
				type Adder = {
					value: number,
					add: (self: Adder, n: number) -> number
				}
				local a: Adder = {
					value = 0,
					add = function(self: Adder, n: number): number
						self.value = self.value + n
						return self.value
					end
				}
				local r: number = a:add(5)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestFunction_Closure(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "closure captures outer variable",
			Code: `
				local x: number = 10
				local function inner(): number
					return x
				end
				local y: number = inner()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "closure modifies outer variable",
			Code: `
				local x: number = 0
				local function increment()
					x = x + 1
				end
				increment()
				local y: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestFunction_Generic(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "generic function identity",
			Code: `
				local function identity<T>(x: T): T
					return x
				end
				local n: number = identity(42)
				local s: string = identity("hello")
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestFunction_OptionalParameterInference tests that parameters with or-default
// patterns are inferred as optional.
func TestFunction_OptionalParameterInference(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "or_default_marks_param_optional",
			Code: `
				local function greet(name, greeting)
					local msg = greeting or "Hello"
					return msg .. ", " .. name
				end
				greet("World")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple_or_defaults",
			Code: `
				local function format(value, prefix, suffix)
					local p = prefix or "["
					local s = suffix or "]"
					return p .. tostring(value) .. s
				end
				format(42)
				format(42, "<")
				format(42, "<", ">")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "explicit_nil_check_optional",
			Code: `
				local function process(data, callback)
					if callback ~= nil then
						callback(data)
					end
				end
				process("test")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "typed_optional_param",
			Code: `
				local function log(msg: string, level: string?)
					local lvl = level or "INFO"
					print(lvl .. ": " .. msg)
				end
				log("hello")
				log("hello", "DEBUG")
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
