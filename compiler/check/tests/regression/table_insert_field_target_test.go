package regression

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// table.insert(t.messages, v) appends to the list held in field messages; it
// writes no key of t itself, so it gives a dynamic t no map shape.
func TestTableInsertIntoFieldKeepsOwnerType(t *testing.T) {
	got := exportedFieldReturn(t, `
local M = {}
function M.f(g)
	local t = g()
	table.insert(t.messages, 1)
	return t
end
return M
`, "f")
	if !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("return = %v, want any", got)
	}
}
