package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestPR55OptionalMetatable(t *testing.T) {
	testutil.RunCases(t, []testutil.Case{
		{Name: "optional metatable cannot supply method", Stdlib: true, WantError: true, Code: `
local M = { __index = { x = 1 } }
local function make(mt: { __index: { x: number } }?)
    return setmetatable({}, mt)
end
local x: number = make(nil).x
`},
		{Name: "nil removes metatable", Stdlib: true, WantError: true, Code: `
local M = { __index = { x = 1 } }
local t = setmetatable({}, M)
local u = setmetatable(t, nil)
local x: number = u.x
`},
		{Name: "present metatable supplies method", Stdlib: true, Code: `
local M = { __index = { x = 1 } }
local x: number = setmetatable({}, M).x
`},
	})
}
