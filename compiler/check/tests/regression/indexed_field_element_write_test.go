package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestIndexedFieldElementWritePaths(t *testing.T) {
	testutil.RunCases(t, []testutil.Case{
		{
			Name: "bracket field",
			Code: `
				type Report = {kinds: {string}}
				local function describe(): Report
					return {kinds = {"a"}}
				end
				local report = describe()
				local before: {string} = report["kinds"]
				report["kinds"][1] = "changed"
				local after: {string} = report.kinds
			`,
		},
		{
			Name: "branch join",
			Code: `
				type Report = {kinds: {string}}
				local function describe(): Report
					return {kinds = {"a"}}
				end
				local function run(choice: boolean): {string}
					local report = describe()
					if choice then
						report.kinds[1] = "left"
					else
						report.kinds[2] = "right"
					end
					local after: {string} = report.kinds
					return after
				end
			`,
		},
		{
			Name: "loop",
			Code: `
				type Report = {kinds: {string}}
				local function describe(): Report
					return {kinds = {"a"}}
				end
				local report = describe()
				for index = 1, 3 do
					local before: {string} = report.kinds
					report.kinds[index] = "changed"
				end
				local after: {string} = report.kinds
			`,
		},
		{
			Name: "guarded optional",
			Code: `
				type Report = {kinds: {string}?}
				local function describe(): Report
					return {kinds = {"a"}}
				end
				local report = describe()
				if report.kinds then
					local before: {string} = report.kinds
					report.kinds[1] = "changed"
					local after: {string} = report.kinds
				end
			`,
		},
	})
}

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
