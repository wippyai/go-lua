package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestImportedFluentRecordPreservesNestedLiteralSelfReturn(t *testing.T) {
	contract := exportModule(t, "contract", `
local contract = {}

function contract.get(_id)
	return {
		with_context = function(self, _context)
			return self
		end,
		with_options = function(self, _options)
			return self
		end,
		open = function(self, _provider_id)
			return {}, nil
		end,
	}, nil
end

return contract
`)

	result := testutil.Check(`
local contract = require("contract")

local provider_contract, err = contract.get("provider")
if err then
	return nil, err
end

local chain = provider_contract:with_context({})
chain = chain:with_options({ retry = true })
local value, open_err = chain:open("p1")
return value, open_err
`, testutil.WithStdlib(), testutil.WithManifest("contract", contract))

	if result.HasError() {
		t.Fatalf("imported fluent record lost nested self-return shape: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
