package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestPR55ReboundIterators(t *testing.T) {
	testutil.RunCases(t, []testutil.Case{
		{Name: "local ipairs shadow", Stdlib: true, WantError: true, Code: `
local ipairs = function(t)
    local function iter() return 1, nil end
    return iter
end
for i, v in ipairs({{ x = 1 }}) do
    local item: { x: number } = v
end
`},
		{Name: "direct nil assignment control", Stdlib: true, WantError: true, Code: `
local v = nil
local item: { x: number } = v
`},
		{Name: "global ipairs reassignment", Stdlib: true, WantError: true, Code: `
ipairs = function(t)
    local function iter() return 1, nil end
    return iter
end
for i, v in ipairs({{ x = 1 }}) do
    local item: { x: number } = v
end
`},
		{Name: "builtin ipairs", Stdlib: true, Code: `
for i, v in ipairs({{ x = 1 }}) do
    local x: number = v.x
end
`},
		{Name: "local pairs shadow key relation", Stdlib: true, WantError: true, Code: `
local pairs = function(t)
    local function iter() return "missing", 1 end
    return iter
end
local t: { [string]: number } = { a = 1 }
for k, v in pairs(t) do
    local x: number = t[k]
end
`},
		{Name: "shadowed ipairs index is string", Stdlib: true, WantError: true, Code: `
local ipairs = function(t)
    local function iter() return "bad", 1 end
    return iter
end
for i, v in ipairs({1}) do
    local n: number = i + 1
end
`},
		{Name: "global pairs reassignment key relation", Stdlib: true, WantError: true, Code: `
pairs = function(t)
    local function iter() return "missing", 1 end
    return iter
end
local t: { [string]: number } = { a = 1 }
for k, v in pairs(t) do
    local x: number = t[k]
end
`},
		{Name: "reassigned ipairs cannot validate enum", Stdlib: true, WantError: true, Code: `
ipairs = function(t)
    local function iter() return 1, "other" end
    return iter
end
local function choose(value: string): "a"
    local choices = { "a" }
    local found = false
    for _, item in ipairs(choices) do
        if item == value then
            found = true
            break
        end
    end
    if not found then return "a" end
    return value
end
local result: "a" = choose("other")
`},
		{Name: "builtin ipairs validates enum", Stdlib: true, Code: `
local function choose(value: string): "a"
    local choices = { "a" }
    local found = false
    for _, item in ipairs(choices) do
        if item == value then
            found = true
            break
        end
    end
    if not found then return "a" end
    return value
end
local result: "a" = choose("a")
`},
	})
}
