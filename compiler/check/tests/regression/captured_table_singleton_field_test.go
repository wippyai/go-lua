package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCapturedTableUnwrittenSingletonFieldKeepsLiteral(t *testing.T) {
	sources := []string{
		`
local M = {S = "a"}
local function get(): "a" return M.S end
return get
`,
		`
local M = {}
M.S = "a"
local function get(): "a" return M.S end
return get, M
`,
		`
local cfg = {mode = "fast"}
local function get(): "fast" return cfg.mode end
print(cfg.mode)
return get
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

func TestCapturedTableWrittenSingletonFieldWidens(t *testing.T) {
	sources := []string{
		// written after the closure is created
		`
local M = {}
M.S = "a"
local function get(): "a" return M.S end
M.S = "b"
return get
`,
		// written by a sibling closure
		`
local M = {}
M.S = "a"
local function get(): "a" return M.S end
local function set() M.S = "b" end
return get, set
`,
		// written through a local alias
		`
local M = {S = "a"}
local A = M
local function get(): "a" return M.S end
A.S = "b"
return get
`,
		// written through a dynamic key
		`
local M = {S = "a"}
local function get(): "a" return M.S end
local function put(k: string) M[k] = "b" end
return get, put
`,
		// written by a function the table escapes to
		`
local M = {S = "a"}
local function set(t) t.S = "b" end
local function get(): "a" return M.S end
set(M)
return get
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
