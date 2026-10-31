package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestOwnerRelationsOnSameGraphCalls(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"local", `
local function lookup(ok: boolean): (string?, string?)
    if ok then return "value", nil end
    return nil, "missing"
end
local value, err = lookup(true)
if err == nil then local s: string = value; return s:sub(1, 1) end
`},
		{"field", `
local service = {}
function service.lookup(ok: boolean): (string?, string?)
    if ok then return "value", nil end
    return nil, "missing"
end
local value, err = service.lookup(true)
if value ~= nil then local s: string = value; return s:sub(1, 1) end
return err
`},
		{"nested", `
local function outer()
    local function lookup(ok: boolean): (string?, string?)
        if ok then return "value", nil end
        return nil, "missing"
    end
    local value, err = lookup(true)
    if err == nil then local s: string = value; return s:sub(1, 1) end
end
return outer()
`},
		{"unannotated", `
local function lookup(ok)
    if ok then return "value", nil end
    return nil, "missing"
end
local value, err = lookup(true)
if err == nil then local s: string = value; return s:sub(1, 1) end
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.source, testutil.WithStdlib())
			if result.HasError() {
				t.Fatalf("lost same-graph return correlation: %v", testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}
