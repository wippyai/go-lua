package regression

import (
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"testing"
)

func TestExtraGateImportedErrorReturnSurvivesModuleView(t *testing.T) {
	module := testutil.CheckAndExport(`
local materialize = {}
function materialize.entry(raw: unknown)
 if type(raw) ~= "table" then return nil, "invalid" end
 local attributes = {}
 for k, v in pairs(raw :: {[string]: string}) do attributes[k] = v end
 return {definition = "yaml", attributes = attributes}
end
return materialize`, "materialize", testutil.WithStdlib())
	if module.HasError() {
		t.Fatal(testutil.ErrorMessages(module.Errors))
	}
	checkBothModes(t, `
local materialize = require("materialize")
type Entry = {id: string}
type Materialized = {definition: string, attributes: {[string]: unknown}?}
type Module = {entry: (Entry) -> (Materialized?, string?)}
local materializer = materialize :: Module
local value, err = materializer.entry({id = "entry"})
if err then return nil end
local definition: string = value.definition
return definition`, "", testutil.WithModule("materialize", module))
}

func TestExtraGateUnprovedErrorReturnNeedsValueGuard(t *testing.T) {
	checkBothModes(t, `
type Materialized = {definition: string}
local function run(materializer: {entry: () -> (Materialized?, string?)})
 local value, err = materializer.entry()
 if err then return nil end
 local definition: string = value.definition
 return definition
end
return run`, "cannot assign string? to string")
}
