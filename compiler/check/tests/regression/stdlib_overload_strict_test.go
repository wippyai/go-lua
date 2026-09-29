package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// checkBothModes expects the same outcome in gradual and strict mode.
func checkBothModes(t *testing.T, code string, want string) {
	t.Helper()
	checkModes(t, code, want, want)
}

// checkModes expects no error when the mode's want is empty, otherwise exactly
// one error containing it.
func checkModes(t *testing.T, code string, wantGradual, wantStrict string) {
	t.Helper()
	for _, strict := range []bool{false, true} {
		name, want := "gradual", wantGradual
		if strict {
			name, want = "strict", wantStrict
		}
		t.Run(name, func(t *testing.T) {
			result := testutil.Check(code, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			messages := testutil.ErrorMessages(result.Diagnostics)
			if want == "" {
				if len(messages) != 0 {
					t.Fatalf("unexpected errors: %v", messages)
				}
				return
			}
			if len(messages) != 1 || !strings.Contains(messages[0], want) {
				t.Fatalf("expected %q, got %v", want, messages)
			}
		})
	}
}

func TestGenericOverloadKeepsArgumentType(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want string
	}{
		{"remove from asserted any list", `
local M = { MAX_UNDO = 10 }
function M.record(t: any, forward_events: any, inverse_events: any)
	local undo_stack = t.undo :: { any }
	undo_stack[#undo_stack + 1] = { forward = forward_events or {}, inverse = inverse_events or {} }
	while #undo_stack > M.MAX_UNDO do table.remove(undo_stack, 1) end
end
return M
`, ""},
		{"remove from any list", `
local function f(list: { any })
	table.remove(list, 1)
	table.remove(list)
end
`, ""},
		{"remove keeps element type", `
local function f(list: { string }): string?
	return table.remove(list, 1)
end
`, ""},
		{"remove rejects non list", `table.remove("x", 1)`, "argument 1:"},
		{"insert into any list", `
local function f(list: { any }, v: any)
	table.insert(list, v)
	table.insert(list, 1, v)
end
`, ""},
		{"random with any bounds", `
local function f(m: integer, n: integer): number
	return math.random() + math.random(m) + math.random(m, n)
end
`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, tt.want) })
	}
}

func TestIndexedWriteKeepsLocalType(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
	}{
		{"asserted list append", `
local function f(u: any)
	local s = u :: { any }
	s[#s + 1] = 1
	local x: { any } = s
end
`},
		{"alias of annotated param", `
local function f(p: { any })
	local s = p
	s[#s + 1] = 1
	local x: { any } = s
end
`},
		{"call result", `
local function g(): { any } return {} end
local s = g()
s[#s + 1] = 1
local x: { any } = s
`},
		{"typed element append", `
local function f(u: any)
	local s = u :: { number }
	s[#s + 1] = 1
	local x: { number } = s
end
`},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, "") })
	}
}
