package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestClassTableUnwrittenSingletonFieldKeepsLiteral(t *testing.T) {
	sources := []string{
		`
local M = {}
M.SCHEMA = "work@3"
M.LEGACY_SCHEMA = "work@2"
type Schema = "work@3" | "work@2"
type Payload = {schema_revision: Schema}
local function seal(value: Payload): Payload return value end
function M.decode(value: unknown): Schema?
    if type(value) ~= "table" then return nil end
    local schema: Schema? = nil
    if value.schema_revision == M.SCHEMA then schema = M.SCHEMA
    elseif value.schema_revision == M.LEGACY_SCHEMA then schema = M.LEGACY_SCHEMA end
    return schema
end
function M.build(): Payload return seal({schema_revision = M.SCHEMA}) end
return M
`,
		`
local M = {SCHEMA = "work@3"}
function M.get(): "work@3" return M.SCHEMA end
return M
`,
		`
local M = {}
M.SCHEMA = "work@3"
function M.get(): "work@3" return M.SCHEMA end
function M.other(): string return M.SCHEMA end
return M
`,
	}
	for _, source := range sources {
		for _, strict := range []bool{false, true} {
			result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			if result.HasError() {
				t.Errorf("strict=%v source=%s: %v", strict, source, testutil.ErrorMessages(result.Errors))
			}
		}
	}
}

func TestClassTableWrittenSingletonFieldWidens(t *testing.T) {
	sources := []string{
		// written by a sibling function
		`
local M = {}
M.S = "a"
function M.set() M.S = "b" end
function M.get(): "a" return M.S end
return M
`,
		// written by a sibling function defined after the reader
		`
local M = {}
M.S = "a"
function M.get(): "a" return M.S end
function M.set() M.S = "b" end
return M
`,
		// written after the reader is defined
		`
local M = {}
M.S = "a"
function M.get(): "a" return M.S end
M.S = "b"
return M
`,
		// written through self in a method of the table
		`
local M = {}
M.S = "a"
function M:set() self.S = "b" end
function M.get(): "a" return M.S end
return M
`,
		// written through a local alias
		`
local M = {}
M.S = "a"
local alias = M
function M.get(): "a" return M.S end
alias.S = "b"
return M
`,
		// written by a function the table escapes to
		`
local M = {}
M.S = "a"
local function set(t) t.S = "b" end
function M.get(): "a" return M.S end
set(M)
return M
`,
		// written through a dynamic key
		`
local M = {}
M.S = "a"
function M.get(): "a" return M.S end
function M.put(k: string, v: string) M[k] = v end
return M
`,
		// read through self in a method
		`
local M = {}
M.S = "a"
function M:get(): "a" return self.S end
return M
`,
	}
	for _, source := range sources {
		for _, strict := range []bool{false, true} {
			result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			messages := strings.Join(testutil.ErrorMessages(result.Errors), "; ")
			if !strings.Contains(messages, `expected "a"`) {
				t.Errorf("strict=%v expected singleton return diagnostic for %s, got %q", strict, source, messages)
			}
		}
	}
}
