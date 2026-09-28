package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestPR55ErrorReturnNeedsProof(t *testing.T) {
	testutil.RunCases(t, []testutil.Case{
		{Name: "nil nil branch", Stdlib: true, WantError: true, Code: `
local function f(n: number): ({ x: number }?, string?)
    if n == 0 then return nil, nil end
    if n == 1 then return nil, "bad" end
    return { x = 1 }, nil
end
local v, err = f(0)
if err then return end
local x: number = v.x
`},
		{Name: "value error branch", Stdlib: true, WantError: true, Code: `
local function f(n: number): ({ x: number }?, string?)
    if n == 0 then return { x = 1 }, "bad" end
    return { x = 1 }, nil
end
local v, err = f(0)
if err then return end
local x: number = v.x
`},
		{Name: "inverse returns", Stdlib: true, Code: `
local function f(n: number)
    if n == 0 then return nil, "bad" end
    return { x = 1 }, nil
end
local v, err = f(1)
if err then return end
local x: number = v.x
`},
		{Name: "annotated body with nil nil", Stdlib: true, WantError: true, Code: `
local f: (number) -> ({ x: number }?, string?) = function(n: number)
    if n == 0 then return nil, nil end
    return { x = 1 }, nil
end
local v, err = f(0)
if err then return end
local x: number = v.x
`},
		{Name: "untyped implicit return", Stdlib: true, WantError: true, Code: `
local function f(n: number)
    if n == 0 then return nil, "bad" end
    if n == 1 then return { x = 1 }, nil end
end
local v, err = f(2)
if err then return end
local x: number = v.x
`},
		{Name: "wrapper drops forwarded error", Stdlib: true, WantError: true, Code: `
local function f(n: number)
    if n == 0 then return nil, "bad" end
    return { x = 1 }, nil
end
local function wrapper(n: number)
    return f(n), nil
end
local v, err = wrapper(0)
if err then return end
local x: number = v.x
`},
		{Name: "wrapper preserves inverse returns", Stdlib: true, Code: `
local function f(n: number)
    if n == 0 then return nil, "bad" end
    return { x = 1 }, nil
end
local function wrapper(n: number)
    return f(n)
end
local v, err = wrapper(1)
if err then return end
local x: number = v.x
`},
	})
}
