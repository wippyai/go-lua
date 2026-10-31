package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// A body record refines a declared {[string]: any} return with its field
// names; a field the body only knows as unknown keeps the declared any.
func TestBroadMapReturnRefinementKeepsDeclaredAnyValues(t *testing.T) {
	result := testutil.Check(`
type Map = { [string]: any }
local function run(ref, input, opts)
    if ref == nil then return nil, "no ref" end
    return { data = input.data, error = opts.error, success = input.ok }, nil
end
local M = {}
M._execute = function(workflow_ref: any, input: any, opts: any): (Map?, string?)
    return run(workflow_ref, input, opts)
end
function M.invoke(ref: any): (any?, string?)
    local descriptor, rerr = M._execute(ref, {}, {})
    if rerr or not descriptor then return nil, rerr or "none" end
    if descriptor.success == false then return nil, descriptor.error or "failed" end
    return descriptor.data, nil
end
return M
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}
