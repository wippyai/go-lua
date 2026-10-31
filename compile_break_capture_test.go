// SPDX-License-Identifier: MPL-2.0

package lua

import "testing"

func TestBreakClosesDepartedCaptures(t *testing.T) {
	for _, loop := range []struct{ name, prefix, suffix string }{
		{"while", "while true do", "end"},
		{"repeat", "repeat", "until true"},
		{"numeric for", "for i = 1, 2 do", "end"},
		{"generic for", "for _, i in ipairs({1, 2}) do", "end"},
	} {
		t.Run(loop.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			code := `
local outer = 1
local function current() return outer end
local read, write, second
` + loop.prefix + `
  do
    local value = 42
    do
      local other = "kept"
      read = function() return value end
      write = function(v) value = v end
      second = function() return other end
      break
    end
  end
` + loop.suffix + `
local reused1, reused2 = 98, 99
outer = 2
assert(read() == 42, "inner capture followed a reused register")
assert(second() == "kept")
assert(current() == 2, "break closed an enclosing capture")
write(77)
assert(read() == 77, "departed closures lost their shared upvalue")
assert(reused1 == 98 and reused2 == 99)
`
			if err := L.DoString(code); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBreakOnlyClosesNearestLoopScope(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
local outer = 0
local function current() return outer end
local saved = {}
for i = 1, 3 do
  outer = outer + 1
  local function iteration() return i end
  while true do
    if i > 0 then
      local value = i * 10
      saved[i] = function() return value end
      break
    end
  end
  assert(iteration() == i, "inner break closed the outer loop capture")
end
assert(current() == 3)
for i = 1, 3 do assert(saved[i]() == i * 10) end
`); err != nil {
		t.Fatal(err)
	}
}

func TestBreakPreservesUnexitedCapture(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
local value = 1
local function current() return value end
while true do
  do break end
end
value = 2
assert(current() == 2)
`); err != nil {
		t.Fatal(err)
	}
}
