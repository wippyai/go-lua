// SPDX-License-Identifier: MPL-2.0

package lua

import "testing"

func TestGotoCloseBoundary(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "forward live capture",
			code: `
local content = ""
local function current() return content end
for _, chunk in ipairs({ "", "Hello" }) do
    if chunk == "" then goto continue end
    content = content .. chunk
    ::continue::
end
assert(content == "Hello")
assert(current() == content, "forward goto detaches a live capture")
`,
		},
		{
			name: "backward live capture",
			code: `
local count = 0
local function current() return count end
::again::
count = count + 1
if count < 4 then goto again end
assert(current() == 4, "backward goto detaches a live capture")
`,
		},
		{
			name: "forward exits captured scope",
			code: `
local outer = 1
local function current() return outer end
local closed
do
    local inner = 10
    closed = function() return inner end
    inner = 11
    goto outside
end
::outside::
local reused = 99
outer = 2
assert(closed() == 11, "exited capture follows a reused register")
assert(current() == 2, "outer capture closes while exiting inner scope")
assert(reused == 99)
`,
		},
		{
			name: "backward exits captured scope repeatedly",
			code: `
local outer = 0
local function current() return outer end
local closed = {}
::again::
outer = outer + 1
do
    local inner = outer * 10
    closed[outer] = function() return inner end
    if outer < 3 then goto again end
end
assert(current() == 3)
for i = 1, 3 do assert(closed[i]() == i * 10) end
`,
		},
		{
			name: "repeated forward jumps",
			code: `
local count = 0
local function current() return count end
for i = 1, 6 do
    goto update
    ::update::
    count = count + 1
    goto continue
    ::continue::
end
assert(count == 6)
assert(current() == count, "repeated forward jumps detach the accumulator")
`,
		},
		{
			name: "nested blocks retain enclosing captures",
			code: `
local outer = 1
local function current() return outer end
do
    local middle = 10
    local function nested() return middle end
    do goto update end
    ::update::
    outer = 2
    middle = 20
    assert(current() == outer)
    assert(nested() == middle, "nested goto closes an enclosing capture")
end
`,
		},
		{
			name: "nested exits close every departed capture",
			code: `
local outer = 1
local function current() return outer end
local first, second
do
    local middle = 10
    first = function() return middle end
    do
        local inner = 20
        second = function() return inner end
        middle = 11
        inner = 21
        goto outside
    end
end
::outside::
local reused1, reused2 = 98, 99
outer = 2
assert(current() == 2)
assert(first() == 11)
assert(second() == 21)
assert(reused1 == 98 and reused2 == 99)
`,
		},
		{
			name: "terminal label closes block locals",
			code: `
local closed
do
    local inner = 10
    closed = function() return inner end
    inner = 11
    goto done
    ::done::
end
local reused = 99
assert(closed() == 11)
assert(reused == 99)
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString(tt.code); err != nil {
				t.Fatal(err)
			}
		})
	}
}
