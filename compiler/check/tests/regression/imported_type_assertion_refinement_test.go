package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
)

func TestImportedTypeAssertionNarrowsArgumentInPlace(t *testing.T) {
	assertModule := testutil.CheckAndExport(`
local M = {}

function M.not_nil(val, msg)
	if val == nil then
		error(msg or "expected non-nil")
	end
	return val
end

function M.is_string(val, msg)
	if type(val) ~= "string" then
		error(msg or "expected string")
	end
	return val
end

function M.ok(val, msg)
	if not val then
		error(msg or "expected truthy")
	end
	return val
end

return M
`, "assert2", testutil.WithStdlib())
	if assertModule.HasError() {
		t.Fatalf("assert module errors: %v", testutil.ErrorMessages(assertModule.Errors))
	}
	summary, ok := assertModule.Manifest.LookupSummary("is_string")
	if !ok || summary == nil {
		t.Fatalf("expected exported is_string summary")
	}
	if !conditionHasConstraint(summary.Ensures, constraint.HasType{
		Path: constraint.ParamPath(0),
		Type: narrow.BuiltinTypeKey("string"),
	}) {
		t.Fatalf("expected is_string summary to ensure $0:string, got %v", summary.Ensures)
	}

	result := testutil.Check(`
local assert = require("assert2")

local function check_denied(_label, msg)
	assert.not_nil(msg, "expected error")
	assert.is_string(msg, "expected string")
	local hit = string.find(msg, "not allowed", 1, true)
		or string.find(msg, "permission", 1, true)
	assert.ok(hit, "expected denial: " .. msg)
end

local result: {http_err: any, funcs_err: any} = {
	http_err = "permission denied",
	funcs_err = "not allowed",
}
check_denied("httpclient edge", result.http_err)
check_denied("funcs edge", result.funcs_err)
	`, testutil.WithStdlib(), testutil.WithModule("assert2", assertModule))
	if result.HasError() {
		t.Fatalf("expected imported is_string assertion to narrow argument in place, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestImportedTypeAssertionNarrowsLocalBeforeStringUse(t *testing.T) {
	assertModule := testutil.CheckAndExport(`
local M = {}

function M.is_string(val, msg)
	if type(val) ~= "string" then
		error(msg or "expected string")
	end
	return val
end

return M
`, "assert2", testutil.WithStdlib())
	if assertModule.HasError() {
		t.Fatalf("assert module errors: %v", testutil.ErrorMessages(assertModule.Errors))
	}

	result := testutil.Check(`
local assert = require("assert2")

local source: {value: any} = {value = "permission denied"}
local value = source.value
assert.is_string(value, "expected string")
return string.find(value, "permission", 1, true)
`, testutil.WithStdlib(), testutil.WithModule("assert2", assertModule))
	if result.HasError() {
		t.Fatalf("expected imported is_string assertion to narrow local before string use, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestAnnotatedImportedTypeAssertionKeepsPrecondition(t *testing.T) {
	assertModule := testutil.CheckAndExport(`
local M = {}

function M.is_string(val: string, msg)
	if type(val) ~= "string" then
		error(msg or "expected string")
	end
	return val
end

return M
`, "typed_assert2", testutil.WithStdlib())
	if assertModule.HasError() {
		t.Fatalf("assert module errors: %v", testutil.ErrorMessages(assertModule.Errors))
	}

	result := testutil.Check(`
local assert = require("typed_assert2")
local value: any = true
assert.is_string(value, "expected string")
`, testutil.WithStdlib(), testutil.WithModule("typed_assert2", assertModule))
	if !result.HasError() {
		t.Fatal("expected annotated imported assertion to keep its string precondition")
	}
	if !strings.Contains(strings.Join(testutil.ErrorMessages(result.Diagnostics), "\n"), "expected string, got any") {
		t.Fatalf("expected annotated assertion precondition error, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func conditionHasConstraint(cond constraint.Condition, want constraint.Constraint) bool {
	for _, got := range cond.AllConstraints() {
		if got.Equals(want) {
			return true
		}
	}
	return false
}
