package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 7) Callback Contextual Typing

func TestCallbackContext_ParameterTypeInferred(t *testing.T) {
	source := `
		function map<T, U>(arr: T[], fn: fun(x: T): U): U[]
			local result: U[] = {}
			for _, v in ipairs(arr) do
				table.insert(result, fn(v))
			end
			return result
		end

		local nums = {1, 2, 3}
		local strs = map(nums, function(x) return tostring(x) end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for callback param inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_ReturnTypeInferred(t *testing.T) {
	source := `
		function filter<T>(arr: T[], pred: fun(x: T): boolean): T[]
			local result: T[] = {}
			for _, v in ipairs(arr) do
				if pred(v) then
					table.insert(result, v)
				end
			end
			return result
		end

		local nums = {1, 2, 3, 4, 5}
		local evens = filter(nums, function(x) return x % 2 == 0 end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for callback return inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_ExplicitTypes(t *testing.T) {
	source := `
		function apply<T>(x: T, fn: fun(v: T): T): T
			return fn(x)
		end

		local result = apply(10, function(v: number): number return v * 2 end)
		local n: number = result
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for explicit callback types, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_MultipleParams(t *testing.T) {
	source := `
		function reduce<T, U>(arr: T[], init: U, fn: fun(acc: U, x: T): U): U
			local result = init
			for _, v in ipairs(arr) do
				result = fn(result, v)
			end
			return result
		end

		local nums = {1, 2, 3}
		local sum = reduce(nums, 0, function(acc, x) return acc + x end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for multi-param callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
