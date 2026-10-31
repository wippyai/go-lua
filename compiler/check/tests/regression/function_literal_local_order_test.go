package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// A function literal synthesized in place infers its locals in dependency
// order: a table local reads the locals assigned before it.
func TestFunctionLiteralLocalsReadEarlierLocals(t *testing.T) {
	result := testutil.Check(`
local M = {}
M.handlers = {
	make = function(p: string)
		local id = p .. "x"
		local node = { id = id }
		return node
	end,
}
local n = M.handlers.make("a")
local s: string = n.id
return M
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}
