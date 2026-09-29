package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// Writing a field into a dynamic table keeps its other fields dynamic: a row
// decoded in place still reads any for a field the code never wrote.
func TestFieldWriteIntoDynamicTableKeepsOtherFieldsAny(t *testing.T) {
	result := testutil.Check(`
local M = {}
local function parse(u)
    if type(u) ~= "table" then return u end
    u.metadata = {}
    return u
end
function M.get(rows: any)
    return parse(rows[1])
end
local up = M.get({})
local s: string = up.mime_type or "x"
return M
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}

// A table-type guard on an unknown value gives the table top; a field write
// extends that top, so a caller of the function still reads the table's
// other fields as any.
func TestFieldWriteIntoGuardedTableTopKeepsOtherFieldsAny(t *testing.T) {
	result := testutil.Check(`
local function parse(u)
    if type(u) ~= "table" then return nil end
    u.metadata = {}
    return u
end
return function(raw)
    local up = parse(raw)
    if not up then return "" end
    local s: string = up.mime_type
    return s
end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
	for _, d := range result.Diagnostics {
		if d.Code == diag.HintImplicitUnknown {
			t.Fatalf("up.mime_type reads any, got: %s", d.Message)
		}
	}
}
