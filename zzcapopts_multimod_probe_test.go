package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZCapOptsMultiMod reproduces the captured-options under-inference using the
// real multi-module wiring (CheckAndExport + WithModule), to isolate whether the
// imported `providers` module is what makes captured resolve to nil.
func TestZZCapOptsMultiMod(t *testing.T) {
	contractSrc := `
local contract = {}
function contract.get(_id)
    return {
        with_context = function(self, _context) return self end,
        with_options = function(self, _options) return self end,
        open = function(self, _provider_id) return {}, nil end,
    }, nil
end
return contract
`
	providersSrc := `
local contract = require("contract")
local providers = { _contract = contract }
function providers.open(provider_id, context_overrides)
    context_overrides = context_overrides or {}
    local provider_contract, err = providers._contract.get("provider")
    if err then return nil, err end
    local chain = provider_contract:with_context({})
    chain = chain:with_options({ retry = context_overrides.retry })
    return chain:open(provider_id)
end
return providers
`
	testSrc := `
local M = {}
function M.is_nil(val, msg)
    if val ~= nil then error(msg or "expected nil", 2) end
end
function M.not_nil(val, msg)
    if val == nil then error(msg or "expected non-nil", 2) end
    return val
end
return M
`
	mainSrc := `
local test = require("test")
local providers = require("providers")
local captured_options = nil
providers._contract = {
    get = function(_contract_id)
        return {
            with_context = function(self, _context) return self end,
            with_options = function(self, opts)
                captured_options = opts
                return self
            end,
            open = function(self, binding_id)
                return { _binding_id = binding_id }, nil
            end,
        }, nil
    end,
}
local instance, err = providers.open("wippy.llm.provider:openai", {
    retry = { max_attempts = 3, initial_delay = 100 },
})
test.is_nil(err, "open should succeed")
assert(instance)
test.not_nil(captured_options, "captured options expected")
test.not_nil(captured_options.retry, "retry expected")
local attempts: number = captured_options.retry.max_attempts
local delay: number = captured_options.retry.initial_delay
return attempts, delay
`

	testMod := testutil.CheckAndExport(testSrc, "test", testutil.WithStdlib())
	for _, e := range testMod.Errors {
		t.Logf("test ERR: %s", e.Message)
	}
	contractMod := testutil.CheckAndExport(contractSrc, "contract", testutil.WithStdlib())
	for _, e := range contractMod.Errors {
		t.Logf("contract ERR: %s", e.Message)
	}
	providersMod := testutil.CheckAndExport(providersSrc, "providers", testutil.WithStdlib(),
		testutil.WithModule("contract", contractMod))
	for _, e := range providersMod.Errors {
		t.Logf("providers ERR: %s", e.Message)
	}
	res := testutil.Check(mainSrc, testutil.WithStdlib(),
		testutil.WithModule("test", testMod),
		testutil.WithModule("contract", contractMod),
		testutil.WithModule("providers", providersMod))
	msgs := testutil.ErrorMessages(res.Diagnostics)
	if len(msgs) == 0 {
		t.Logf("main NO DIAG")
	}
	for _, m := range msgs {
		t.Logf("main DIAG: %s", m)
	}
}
