package functions

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestMethodCalls tests method call syntax and type checking.
func TestMethodCalls(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "string method",
			Code: `
				local s = "hello"
				local u: string = s:upper()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string method chain",
			Code: `
				local s = "  hello  "
				local result: string = s:gsub(" ", ""):upper()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "custom method with self",
			Code: `
				local obj = {
					value = 0,
					increment = function(self)
						self.value = self.value + 1
					end
				}
				obj:increment()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "method on union requires narrowing",
			Code: `
				function f(x: string | number)
					x:upper()
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTablePatterns tests common table usage patterns.
func TestTablePatterns(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "nested table access",
			Code: `
				local config = {
					server = {
						host = "localhost",
						port = 8080
					}
				}
				local port: number = config.server.port
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "table insert preserves type",
			Code: `
				local arr: {number} = {}
				table.insert(arr, 1)
				table.insert(arr, 2)
				local n: number = arr[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "table remove returns element",
			Code: `
				local arr: {string} = {"a", "b"}
				local s: string? = table.remove(arr)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "table concat returns string",
			Code: `
				local arr = {"a", "b", "c"}
				local s: string = table.concat(arr, ",")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "mixed array fails",
			Code: `
				local arr: {number} = {1, "two", 3}
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestPcallXpcall tests protected call functions.
func TestPcallXpcall(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "pcall returns boolean and results",
			Code: `
				local ok, result = pcall(function() return 42 end)
				local b: boolean = ok
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "pcall with args",
			Code: `
				local ok, result = pcall(tostring, 42)
				local b: boolean = ok
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "xpcall with handler",
			Code: `
				local ok, result = xpcall(function() return "test" end, function(err) return err end)
				local b: boolean = ok
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestGotoLabel tests goto and label statements.
func TestGotoLabel(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "simple goto forward",
			Code: `
				goto target
				::target::
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "goto backward",
			Code: `
				::start::
				goto start
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "goto undefined label",
			Code: `
				goto undefined
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "duplicate label",
			Code: `
				::dup::
				::dup::
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "break outside loop",
			Code: `
				break
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "break in function inside loop",
			Code: `
				while true do
					local f = function() break end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestHigherOrderFunctions tests functions that take or return functions.
func TestHigherOrderFunctions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "map function",
			Code: `
				local function map(arr: {number}, fn: (number) -> number): {number}
					local result: {number} = {}
					for i, v in ipairs(arr) do
						result[i] = fn(v)
					end
					return result
				end
				local doubled = map({1, 2, 3}, function(x: number): number return x * 2 end)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "callback with closure",
			Code: `
				local function createCounter(): () -> number
					local count = 0
					return function(): number
						count = count + 1
						return count
					end
				end
				local counter = createCounter()
				local first: number = counter()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function composition",
			Code: `
				local function compose(f: (number) -> number, g: (number) -> number): (number) -> number
					return function(x: number): number
						return f(g(x))
					end
				end
				local function double(x: number): number return x * 2 end
				local function addOne(x: number): number return x + 1 end
				local composed = compose(double, addOne)
				local result: number = composed(5)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "curried function",
			Code: `
				local function createAdder(x: number): (number) -> number
					return function(y: number): number
						return x + y
					end
				end
				local add5 = createAdder(5)
				local result: number = add5(3)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestClosures tests closure behavior.
func TestClosures(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "closure captures local",
			Code: `
				local x = 10
				local function getX(): number
					return x
				end
				local result: number = getX()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "closure modifies captured",
			Code: `
				local function counter()
					local count = 0
					return {
						inc = function() count = count + 1 end,
						get = function(): number return count end
					}
				end
				local c = counter()
				c.inc()
				local val: number = c.get()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested closure 3 levels",
			Code: `
				local function outer(a: number): (number) -> (number) -> number
					return function(b: number): (number) -> number
						return function(c: number): number
							return a + b + c
						end
					end
				end
				local result: number = outer(1)(2)(3)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestStringMethods tests string metatable method calls.
func TestStringMethods(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "string_match_method",
			Code: `
				local function parse_pair(s: string): (string, string)
					local k, v = s:match("(%w+)=(%w+)")
					return k or "", v or ""
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string_sub_method",
			Code: `
				local function first_char(s: string): string
					return s:sub(1, 1)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string_gsub_method",
			Code: `
				local function normalize(s: string): string
					return s:gsub("%s+", " ")
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string_find_method",
			Code: `
				local function contains(s: string, pattern: string): boolean
					return s:find(pattern) ~= nil
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string_len_method",
			Code: `
				local function is_empty(s: string): boolean
					return s:len() == 0
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string_format_method",
			Code: `
				local function fmt(template: string, value: number): string
					return template:format(value)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "chained_string_methods",
			Code: `
				local function clean(s: string): string
					return s:lower():gsub("%s+", "")
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string_rep_method",
			Code: `
				local function repeat_str(s: string, n: integer): string
					return s:rep(n)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string_reverse_method",
			Code: `
				local function reverse(s: string): string
					return s:reverse()
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string_byte_method",
			Code: `
				local function first_byte(s: string): number
					return s:byte(1)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestVariadicFunctions tests variadic function definitions.
func TestVariadicFunctions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "simple variadic",
			Code: `
				local function sum(...: number): number
					local result = 0
					for _, v in ipairs({...}) do
						result = result + v
					end
					return result
				end
				local total: number = sum(1, 2, 3)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "variadic with fixed params",
			Code: `
				local function printf(fmt: string, ...: any)
					print(string.format(fmt, ...))
				end
				printf("Hello %s", "world")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "select with variadic",
			Code: `
				local function test(...: number)
					local count = select("#", ...)
					local first = select(1, ...)
				end
				test(1, 2, 3)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
