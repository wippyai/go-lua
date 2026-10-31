package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
)

func TestImportedPresentAliasRetainsFieldIdentity(t *testing.T) {
	exported := testutil.CheckAndExport(`
type Options = {size: integer}
local M = {}
function M.options(): Options? return {size = 1} end
return M`, "provider", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatal(testutil.ErrorMessages(exported.Errors))
	}
	options := []testutil.Option{
		testutil.WithManifest("provider", exported.Manifest),
		// Runtime manifests can retain a local reference in a containing alias.
		testutil.WithTypes(map[string]typ.Type{"Options": typ.NewRef("", "Options")}),
	}
	checkBothModes(t, `
local provider = require("provider")
local function run()
 local options = provider.options()
 if type(options) ~= "table" then return nil end
 local ctx: {options: Options} = {options = options}
 return ctx
end
return run`, "", options...)
	checkBothModes(t, `
local provider = require("provider")
local function run()
 local options = provider.options()
 local ctx: {options: Options} = {options = options}
 return ctx
end
return run`, "cannot assign", options...)
}
