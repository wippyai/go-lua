package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func TestCapturedCalleeIterationShapes(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "annotated map at module scope",
			source: `
local MIME_TYPES: {[string]: string} = { txt = "text/plain" }
local function lookup(ext: string)
    if MIME_TYPES[ext] then return MIME_TYPES[ext] end
    return "application/octet-stream"
end
local mime_type: string? = lookup("txt")
return mime_type
`,
		},
		{
			name: "annotated map through captured callee",
			source: `
local MIME_TYPES: {[string]: string} = { txt = "text/plain" }
local function lookup(ext: string)
    if MIME_TYPES[ext] then return MIME_TYPES[ext] end
    return "application/octet-stream"
end
return function()
    local mime_type: string? = lookup("txt")
    return mime_type
end
`,
		},
		{
			name: "guarded table at module scope",
			source: `
local function parse(u)
    if type(u) ~= "table" then return nil end
    u.metadata = {}
    return u
end
local up = parse({})
if not up then return "" end
local s: string = up.mime_type
return s
`,
		},
		{
			name: "guarded table through captured callee",
			source: `
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
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.source, testutil.WithStdlib())
			if result.HasError() {
				t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
			}
			for _, d := range result.Diagnostics {
				if d.Code == diag.HintImplicitUnknown {
					t.Fatalf("lost captured type evidence: %s", d.Message)
				}
			}
		})
	}
}
