package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZLookupMinProbe reproduces the lookup-table-cast root in isolation:
// an annotated {[number]: string} local seeded with {} then written by literal
// integer keys, then read by a non-literal number. Confirms whether the index
// read errors and whether the annotation is preserved. Read-only probe.
func TestZZLookupMinProbe(t *testing.T) {
	cases := map[string]string{
		"annotated-map-literal-writes": `
type StatusCodeMap = {[number]: string}
local status_codes: StatusCodeMap = {}
status_codes[400] = "invalid_request"
status_codes[401] = "authentication"
local function read(code: number): string
    return status_codes[code] or "x"
end
return read
`,
		"annotated-map-no-writes": `
type StatusCodeMap = {[number]: string}
local status_codes: StatusCodeMap = {}
local function read(code: number): string
    return status_codes[code] or "x"
end
return read
`,
		"string-key-analogue": `
type ErrorTypeMap = {[string]: string}
local error_types: ErrorTypeMap = {}
error_types["a"] = "invalid_request"
local function read(k: string): string
    return error_types[k] or "x"
end
return read
`,
		"module-field-int": `
type StatusCodeMap = {[number]: string}
local M = {}
local status_codes: StatusCodeMap = {}
status_codes[400] = "invalid_request"
status_codes[401] = "authentication"
M.status_codes = status_codes
function M.map_status_code(code: number): string
    return M.status_codes[code] or "server_error"
end
return M
`,
		"module-field-str": `
type ErrorTypeMap = {[string]: string}
local M = {}
local error_types: ErrorTypeMap = {}
error_types["a"] = "invalid_request"
M.error_types = error_types
function M.map_error(k: string): string
    return M.error_types[k] or "server_error"
end
return M
`,
	}

	opt := testutil.WithCheckOption(check.WithCanonicalFlow())
	for name, src := range cases {
		mod := testutil.CheckAndExport(src, "m_"+name, opt)
		t.Logf("=== %s: %d errors ===", name, len(mod.Errors))
		for _, d := range mod.Errors {
			t.Logf("   %s:%d:%d %s", d.Position.File, d.Position.Line, d.Position.Column, d.Message)
		}
	}
}
