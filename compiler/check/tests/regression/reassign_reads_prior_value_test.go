package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// `x = f(x)` reads x before it writes x: the read sees the guarded value, not
// the value the assignment produces.
func TestReassignmentReadsPriorValue(t *testing.T) {
	result := testutil.Check(`
local function trim(body: any): string?
    local title = body.title
    if type(title) ~= "string" then
        return nil
    end
    title = title:match("^%s*(.-)%s*$")
    return title
end
return trim
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}
