package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestNarrowing_TypeofGuard(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "typeof narrowing string",
			Code: `
				local x: string | number = "hello"
				if type(x) == "string" then
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "typeof narrowing number",
			Code: `
				local x: string | number = 42
				if type(x) == "number" then
					local n: number = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "typeof narrowing excludes other",
			Code: `
				local x: string | number = 42
				if type(x) == "string" then
					local n: number = x
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestNarrowing_NilCheck(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "nil check narrows optional",
			Code: `
				local x: string? = nil
				if x ~= nil then
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nil check in else branch",
			Code: `
				local x: string? = nil
				if x == nil then
					local s: nil = x
				else
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "truthiness narrows nil",
			Code: `
				local x: string? = "test"
				if x then
					local s: string = x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestNarrowing_Discriminator(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "tagged union discriminator",
			Code: `
				type Dog = {kind: "dog", bark: () -> ()}
				type Cat = {kind: "cat", meow: () -> ()}
				type Animal = Dog | Cat

				local function speak(a: Animal)
					if a.kind == "dog" then
						a.bark()
					else
						a.meow()
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "discriminator wrong method",
			Code: `
				type Dog = {kind: "dog", bark: () -> ()}
				type Cat = {kind: "cat", meow: () -> ()}
				type Animal = Dog | Cat

				local function speak(a: Animal)
					if a.kind == "dog" then
						a.meow()
					end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestNarrowing_NestedField(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "narrowing union by channel field",
			Code: `
				type ChanInt = {__tag: "int"}
				type ChanStr = {__tag: "str"}
				type SelResult =
					{channel: ChanInt, value: {error: string}, ok: boolean} |
					{channel: ChanStr, value: {data: number}, ok: boolean}

				local function get_result(a: ChanInt, b: ChanStr): SelResult
					return {channel = a, value = {error = "oops"}, ok = true}
				end

				local function f(ch1: ChanInt, ch2: ChanStr)
					local result = get_result(ch1, ch2)
					if result.channel == ch1 then
						local e: string = result.value.error
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestNarrowing_Assert(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "assert narrows to truthy",
			Code: `
				local x: string? = "test"
				assert(x)
				local s: string = x
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "assert with condition",
			Code: `
				local x: string | number = 42
				assert(type(x) == "number")
				local n: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestNarrowing_FieldExistence(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "field existence narrows union",
			Code: `
				type Event = {kind: string, error: string?}
				type Message = {topic: string, payload: any}
				type Timer = {elapsed: number}
				type SelectResult = Event | Message | Timer

				local function get_result(): SelectResult
					return {kind = "exit", error = nil}
				end

				local function f()
					local result = get_result()
					if result.kind then
						local k: string = result.kind
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestNarrowing_OptionalNestedIf(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "simple if narrows optional record",
			Code: `
				type Error = {kind: string, message: string}
				local function test(): Error?
					return {kind = "test", message = "msg"}
				end
				local err = test()
				if err then
					local msg = err.message
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested if preserves narrowing record",
			Code: `
				type Error = {kind: string, message: string}
				local function test(): Error?
					return {kind = "test", message = "msg"}
				end
				local err = test()
				local flag = true
				if err then
					if flag then
						local msg = err.message
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "simple if narrows optional for method call",
			Code: `
				type Error = {kind: (self: Error) -> string, message: (self: Error) -> string}
				local function test(): Error?
					return nil
				end
				local err = test()
				if err then
					local msg = err:message()
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested if preserves narrowing for method call",
			Code: `
				type Error = {kind: (self: Error) -> string, message: (self: Error) -> string}
				local function test(): Error?
					return nil
				end
				local err = test()
				local flag = true
				if err then
					if flag then
						local msg = err:message()
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "deeply nested if preserves narrowing for method call",
			Code: `
				type Error = {kind: (self: Error) -> string, message: (self: Error) -> string, retryable: (self: Error) -> boolean}
				local function test(): Error?
					return nil
				end
				local err = test()
				local a, b, c = true, true, true
				if err then
					if a then
						if b then
							if c then
								local k = err:kind()
								local m = err:message()
								local r = err:retryable()
							end
						end
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple method calls after nil check",
			Code: `
				type Error = {kind: (self: Error) -> string, message: (self: Error) -> string, retryable: (self: Error) -> boolean}
				local function test(): Error?
					return nil
				end
				local err = test()
				if err then
					local kind = err:kind()
					if kind == "network" then
						local retryable = err:retryable()
						local message = err:message()
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
