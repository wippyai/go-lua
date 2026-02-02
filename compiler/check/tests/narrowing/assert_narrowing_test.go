package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestAssertNarrowing tests assert function narrowing.
func TestAssertNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "assert narrows truthy",
			Code: `
				function f(x: string?)
					assert(x)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assert with message",
			Code: `
				function f(x: number?)
					assert(x, "x must not be nil")
					local n: number = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assert with condition",
			Code: `
				function f(x: number)
					assert(x > 0, "x must be positive")
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "error terminates flow",
			Code: `
				function f(x: string?): string
					if x == nil then
						error("x is nil")
					end
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestAssertLibraryNarrowing tests custom assert library patterns.
func TestAssertLibraryNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "assert.not_nil narrows",
			Code: `
				local assert = {
					not_nil = function(val: any, msg: string?)
						if val == nil then error(msg or "assertion failed") end
					end
				}
				function process(x: string?)
					assert.not_nil(x)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assert.is_nil narrows inverse",
			Code: `
				local assert = {
					is_nil = function(val: any, msg: string?)
						if val ~= nil then error(msg or "expected nil") end
					end
				}
				function process(x: string?, err: string?)
					assert.is_nil(err)
					local s: string = x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "assert.eq pattern",
			Code: `
				local assert = {
					eq = function(a: any, b: any, msg: string?)
						if a ~= b then error(msg or "not equal") end
					end
				}
				function test()
					local x = 1
					assert.eq(x, 1)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assert function terminates flow",
			Code: `
				local assert = {
					not_nil = function(val: any, msg: string?)
						if val == nil then error(msg or "nil") end
					end
				}
				function getOrFail(x: string?): string
					assert.not_nil(x, "x must not be nil")
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "chained assertions",
			Code: `
				local assert = {
					not_nil = function(val: any, msg: string?)
						if val == nil then error(msg or "nil") end
					end
				}
				function process(a: string?, b: number?)
					assert.not_nil(a, "a")
					assert.not_nil(b, "b")
					local s: string = a
					local n: number = b
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assert on field path",
			Code: `
				local assert = {
					not_nil = function(val: any, msg: string?)
						if val == nil then error(msg or "nil") end
					end
				}
				function process(obj: {stream: {read: () -> string}?})
					assert.not_nil(obj.stream)
					local s: string = obj.stream:read()
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestErrorReturnPattern tests the result, err return pattern.
func TestErrorReturnPattern(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "result, err pattern with check",
			Code: `
				local function getData(): (string?, string?)
					return "data", nil
				end
				local data, err = getData()
				if err then
					error(err)
				end
				local s: string = data
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "result, err pattern without check fails",
			Code: `
				local function getData(): (string?, string?)
					return "data", nil
				end
				local data, err = getData()
				local s: string = data
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "multiple error returns",
			Code: `
				local function process(x: number): (number?, string?)
					if x < 0 then
						return nil, "negative"
					end
					return x * 2, nil
				end
				local result, err = process(5)
				if err ~= nil then
					return
				end
				local n: number = result
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "error propagation",
			Code: `
				local function inner(): (string?, string?)
					return nil, "error"
				end
				local function outer(): (string?, string?)
					local result, err = inner()
					if err ~= nil then
						return nil, err
					end
					return result, nil
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestAssertWrapperNarrowing tests that functions wrapping native assert() propagate constraints.
func TestAssertWrapperNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "wrapper calling assert narrows",
			Code: `
				local function assertNotNil(val: any)
					assert(val, "value must not be nil")
				end
				function process(x: string?)
					assertNotNil(x)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "wrapper with custom message narrows",
			Code: `
				local function must(val: any, msg: string)
					assert(val, "must: " .. msg)
				end
				function process(x: number?)
					must(x, "x is required")
					local n: number = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "wrapper on field path narrows",
			Code: `
				local function assertNotNil(val: any)
					assert(val, "value must not be nil")
				end
				function process(obj: {data: string?})
					assertNotNil(obj.data)
					local s: string = obj.data
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "chained wrappers narrow",
			Code: `
				local function assertNotNil(val: any)
					assert(val, "not nil")
				end
				function process(a: string?, b: number?)
					assertNotNil(a)
					assertNotNil(b)
					local s: string = a
					local n: number = b
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "wrapper in module table narrows",
			Code: `
				local check = {}
				function check.notNil(val: any, msg: string?)
					assert(val, msg or "value is nil")
				end
				function process(x: string?)
					check.notNil(x)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested wrapper calls narrow",
			Code: `
				local function innerAssert(val: any, msg: string)
					assert(val, msg)
				end
				local function outerAssert(val: any)
					innerAssert(val, "outer: value is nil")
				end
				function process(x: string?)
					outerAssert(x)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "wrapper without assert does not narrow",
			Code: `
				local function maybeCheck(val: any)
					if val == nil then
						print("warning: nil value")
					end
				end
				function process(x: string?)
					maybeCheck(x)
					local s: string = x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "conditional wrapper does not narrow",
			Code: `
				local function conditionalAssert(val: any, check: boolean)
					if check then
						assert(val, "value is nil")
					end
				end
				function process(x: string?)
					conditionalAssert(x, true)
					local s: string = x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestInferredContracts tests that functions infer narrowing effects.
func TestInferredContracts(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "function with internal error infers termination",
			Code: `
				local function assertNotNil(val: any)
					if val == nil then
						error("value is nil")
					end
				end
				function process(x: string?)
					assertNotNil(x)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function returning non-nil infers return type",
			Code: `
				local function getOrDefault(val: string?, default: string): string
					if val == nil then
						return default
					end
					return val
				end
				local s: string = getOrDefault(nil, "default")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "error in all branches terminates",
			Code: `
				local function fail(msg: string)
					error(msg)
				end
				function process(x: string?): string
					if x == nil then
						fail("x is nil")
					end
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "conditional error does not fully terminate",
			Code: `
				local function maybeError(cond: boolean)
					if cond then
						error("condition was true")
					end
				end
				function process(x: string?)
					maybeError(x == nil)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assert eq narrows to intersection",
			Code: `
				local function assertEq(a: any, b: any)
					if a ~= b then error("not equal") end
				end
				function process(x: string | number, y: string)
					assertEq(x, y)
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
