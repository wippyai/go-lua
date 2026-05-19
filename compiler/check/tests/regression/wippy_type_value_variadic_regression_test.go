package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestWippyTypeValueAliasesSurviveModuleExportAndRuntimeIs(t *testing.T) {
	inner := testutil.CheckAndExport(`
type InnerID = string @min_len(2)
type Flag = "hot" | "cold" | "warm"
type Flags = {Flag} @min_len(1) @max_len(3)
type Inner = {id: InnerID, flags: Flags}

local function make_inner(id: string, flags: Flags): Inner
	return ({id = id, flags = flags} as Inner)
end

return {
	InnerID = InnerID,
	Flag = Flag,
	Flags = Flags,
	Inner = Inner,
	make_inner = make_inner,
}
`, "inner", testutil.WithStdlib())
	if inner.HasError() {
		t.Fatalf("unexpected inner module errors: %v", testutil.ErrorMessages(inner.Errors))
	}

	outer := testutil.CheckAndExport(`
local inner = require("inner")

type Label = string @min_len(1) @max_len(8)
type Outer = {inner: inner.Inner, label: Label, meta?: {[string]: string}}
type OuterList = {Outer} @min_len(1)

local function wrap(val: inner.Inner, label: string): Outer
	return ({inner = val, label = label} as Outer)
end

return {
	Inner = inner.Inner,
	Label = Label,
	Outer = Outer,
	OuterList = OuterList,
	wrap = wrap,
}
`, "outer", testutil.WithStdlib(), testutil.WithModule("inner", inner))
	if outer.HasError() {
		t.Fatalf("unexpected outer module errors: %v", testutil.ErrorMessages(outer.Errors))
	}

	result := testutil.Check(`
local inner = require("inner")
local outer = require("outer")

local function main(): boolean
	local candidate = inner.make_inner("ok", {"warm"})
	local _, err = outer.Inner:is(candidate)
	if err then
		return false
	end

	local wrapped = outer.wrap(inner.make_inner("ok", {"warm"}), "tag")
	local id: string = wrapped.inner.id
	return id == "ok"
end
`, testutil.WithStdlib(), testutil.WithModule("inner", inner), testutil.WithModule("outer", outer))
	if result.HasError() {
		t.Fatalf("expected exported type values and nested aliases to remain precise, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestWippyAnnotatedVariadicFunctionFactKeepsVarargs(t *testing.T) {
	result := testutil.Check(`
local function sum(...: number): number
	local result = 0
	for _, v in ipairs({...}) do
		result = result + v
	end
	return result
end

local function format(fmt: string, ...: any): string
	return string.format(fmt, ...)
end

local function count(...: any): number
	return select("#", ...)
end

local function main(): boolean
	local s1: number = sum(1, 2, 3)
	local f1: string = format("x=%d y=%d", 10, 20)
	local f2: string = format("hello %s", "world")
	local c1: number = count(1, 2, 3, 4, 5)
	return s1 == 6 and f1 == "x=10 y=20" and f2 == "hello world" and c1 == 5
end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit varargs to survive function-fact refinement, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
