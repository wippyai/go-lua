package types

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestTypeCast_StringCast(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "string cast from any",
			Code: `
				local x: any = "hello"
				local s = string(x)
				local len = #s
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_BooleanCast(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "boolean cast from any",
			Code: `
				local x: any = true
				local b = boolean(x)
				if b then
					local n = 1
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_StringLibStillWorks(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "string library methods",
			Code: `
				local s = "hello"
				local upper = string.upper(s)
				local lower = string.lower(s)
				local len = string.len(s)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_BothCastAndLibrary(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "cast and library together",
			Code: `
				local x: any = "hello"
				local s = string(x)
				local upper = string.upper(s)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_ChainedCast(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "chained casts from any fields",
			Code: `
				local data: any = { name = "test", count = 42 }
				local name = string(data.name)
				local count = integer(data.count)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_AliasTypes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "int and bool aliases",
			Code: `
				local n = int(42)
				local b = bool(true)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_CastInConcat(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "string cast in concatenation",
			Code: `
				local prefix: any = "Hello, "
				local name: any = "World"
				local greeting = string(prefix) .. string(name)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_MultipleCastsInStatement(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "multiple casts in one statement",
			Code: `
				local data: any = {a = "1", b = 2, c = true}
				local s, n, b = string(data.a), integer(data.b), boolean(data.c)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_IntegerCast(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "integer cast assigned to typed variable",
			Code: `
				local x: any = 100
				local n: integer = integer(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast in arithmetic",
			Code: `
				local x: any = 100
				local n = integer(x) + 50
				local m: integer = n
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast passed to typed function",
			Code: `
				local function double(n: integer): integer
					return n * 2
				end
				local x: any = 5
				local result = double(integer(x))
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast in comparison",
			Code: `
				local x: any = 100
				local cmp = integer(x) > 50
				local b: boolean = cmp
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast return from function",
			Code: `
				local function parse(s: any): integer
					return integer(s)
				end
				local n: integer = parse("42")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer cast in table field",
			Code: `
				local x: any = 100
				local t: {count: integer} = {count = integer(x)}
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_NumberCast(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "number cast assigned to typed variable",
			Code: `
				local x: any = 3.14
				local n: number = number(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number cast in arithmetic",
			Code: `
				local x: any = 100
				local n = number(x) * 2.5
				local m: number = n
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number cast passed to typed function",
			Code: `
				local function half(n: number): number
					return n / 2
				end
				local x: any = 10
				local result = half(number(x))
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_TostringReturn(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "tostring result assigned to string variable",
			Code: `
				local x: any = 42
				local s: string = tostring(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "tostring result in concatenation",
			Code: `
				local x: any = 42
				local s: string = "value: " .. tostring(x)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "tostring on number",
			Code: `
				local n: number = 3.14
				local s: string = tostring(n)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "tostring chained with integer cast",
			Code: `
				local x: any = 100
				local s: string = tostring(integer(x))
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_CastInReturn(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "integer cast in function return",
			Code: `
				local function getInt(data: any): integer
					return integer(data)
				end
				local result: integer = getInt(42)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number cast in function return",
			Code: `
				local function getNum(data: any): number
					return number(data)
				end
				local result: number = getNum(3.14)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_CastInArithmetic(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "multiple integer casts in arithmetic",
			Code: `
				local a: any = 10
				local b: any = 20
				local sum = integer(a) + integer(b)
				local result: integer = sum
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "mixed casts in arithmetic",
			Code: `
				local a: any = 10
				local b: any = 2.5
				local result = integer(a) + number(b)
				local n: number = result
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_CastInTableConstructor(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "casts in table constructor",
			Code: `
				local raw: any = {name = "test", count = 42}
				local config: {name: string, count: integer} = {
					name = tostring(raw.name),
					count = integer(raw.count)
				}
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_TonumberOptional(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "tonumber returns optional",
			Code: `
				local s = "123"
				local n = tonumber(s)
				if n then
					local x: number = n
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "tonumber with base",
			Code: `
				local s = "FF"
				local n = tonumber(s, 16)
				if n then
					local x: number = n
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_CustomType(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "custom type cast",
			Code: `
				type Point = {x: number, y: number}
				local v: any = {x = 1, y = 2}
				local p = Point(v)
				local sum = p.x + p.y
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested record type cast",
			Code: `
				type Address = {street: string, city: string}
				type Person = {name: string, address: Address}
				local data: any = {name = "Alice", address = {street = "123 Main", city = "NYC"}}
				local p = Person(data)
				local name = p.name
				local city = p.address.city
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "array type cast",
			Code: `
				type Numbers = {integer}
				local data: any = {1, 2, 3}
				local nums = Numbers(data)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "generic record type cast",
			Code: `
				type StringResult = {ok: boolean, value: string}
				local data: any = {ok = true, value = "success"}
				local r = StringResult(data)
				local v = r.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "cast from method return",
			Code: `
				type Data = {value: string}
				local obj = {
					getData = function(self): any
						return {value = "test"}
					end
				}
				local d = Data(obj:getData())
				local v = d.value
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTypeCast_TypeIsMethod(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "Type:is basic pattern",
			Code: `
				type Point = {x: number, y: number}
				local function validate(data: any)
					local val, err = Point:is(data)
					if val ~= nil then
						local p: {x: number, y: number} = data
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type:is result stored then checked",
			Code: `
				type Point = {x: number, y: number}
				local function validate(data: any)
					local val = Point:is(data)
					if val then
						local sum = val.x + val.y
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type:is direct condition narrows",
			Code: `
				type Point = {x: number, y: number}
				local function validate(data: any)
					if Point:is(data) then
						local p: {x: number, y: number} = data
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type:is with field access",
			Code: `
				type Point = {x: number, y: number}
				local v: any = {x = 1, y = 2}
				local p, err = Point:is(v)
				if p then
					local sum = p.x + p.y
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "Type:is falsy check should fail",
			Code: `
				type Point = {x: number, y: number}
				local function validate(data: any)
					local ok = Point:is(data)
					if not ok then
						local p: {x: number, y: number} = data
					end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "Type:is with not condition should fail",
			Code: `
				type Point = {x: number, y: number}
				local function validate(data: any)
					if not Point:is(data) then
						local p: {x: number, y: number} = data
					end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
