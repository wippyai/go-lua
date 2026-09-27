package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Minimal omitted-mode variant of kickside/platform/transfer/src/bundle_test.lua:11-23,137-139.
func TestLiteralOverloadDoesNotAcceptOmittedArgument(t *testing.T) {
	result := testutil.Check(`
local function open(mode)
    if mode == "w" then return { write = function() end } end
    return nil
end
local h = open()
h:write()
`, testutil.WithStdlib())
	if len(result.Errors) != 1 || result.Errors[0].Position.Line != 7 || result.Errors[0].Message != "cannot call method on optional value without nil check" {
		t.Fatalf("omitted mode must retain the nil obligation: %v", testutil.ErrorMessages(result.Errors))
	}
}
