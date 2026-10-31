package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestTruthyErrorReturnDoesNotInferFromFalseSuccess(t *testing.T) {
	producer := testutil.CheckAndExport(`
local M = {}
function M.lookup(x: boolean): (boolean?, string?)
    if x then return false, nil end
    return nil, "missing"
end
return M
`, "lookup", testutil.WithStdlib())
	if producer.HasError() {
		t.Fatalf("unexpected producer errors: %v", testutil.ErrorMessages(producer.Errors))
	}
	consumer := testutil.Check(`
local lookup = require("lookup")
local function need_string(s: string) return s end
local ok, err = lookup.lookup(true)
if not ok then need_string(err) end
`, testutil.WithStdlib(), testutil.WithModule("lookup", producer))
	if !consumer.HasError() || !strings.Contains(strings.Join(testutil.ErrorMessages(consumer.Diagnostics), " | "), "string?") {
		t.Fatalf("false,nil is reachable, so error must remain optional: %v", testutil.ErrorMessages(consumer.Diagnostics))
	}
}
