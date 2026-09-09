package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: writing an element through a record field path
// (described.kinds[1] = ...) widens the list held by that field, not the record
// that owns it. Earlier reads of the field must still see the field type.
func TestIndexedFieldElementWriteKeepsFieldType(t *testing.T) {
	source := `
		type Report = {kinds: {string}, sources: {string}}

		local function describe(): Report
			return {kinds = {"a", "b"}, sources = {"x"}}
		end

		local function same(left: {string}, right: {string})
			for index, item in ipairs(left) do
				assert(item == right[index])
			end
		end

		local function run()
			local described = describe()
			same(described.kinds, {"a", "b"})
			same(described.sources, {"x"})
			described.kinds[1] = "changed"
		end

		return {run = run}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard: the same shape with the record type defined in another
// module, which is how cross-module record types reach the call site.
func TestIndexedFieldElementWriteKeepsFieldTypeAcrossModules(t *testing.T) {
	libSource := `
		type Report = {kinds: {string}, sources: {string}}

		local M = {}

		function M.describe(): Report
			return {kinds = {"a", "b"}, sources = {"x"}}
		end

		return M
	`

	lib := testutil.CheckAndExport(libSource, "lib", testutil.WithStdlib())
	if lib.HasError() {
		t.Fatalf("expected no errors in lib, got: %v", lib.Errors)
	}

	source := `
		local lib = require("lib")

		local function same(left: {string}, right: {string})
			for index, item in ipairs(left) do
				assert(item == right[index])
			end
		end

		local function run()
			local described = lib.describe()
			same(described.kinds, {"a", "b"})
			same(described.sources, {"x"})
			described.kinds[1] = "changed"
		end

		return {run = run}
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("lib", lib))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
