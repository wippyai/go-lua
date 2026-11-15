package lua

import (
	"math"
	"testing"
)

// Expectations come from the Lua 5.4 reference interpreter.
func TestMathHugeIsInfinity(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`return math.huge`); err != nil {
		t.Fatal(err)
	}
	if got := L.Get(-1); got != LNumber(math.Inf(1)) {
		t.Fatalf("math.huge = %v, want +Inf", got)
	}

	if err := L.DoString(`
assert(math.type(math.huge) == "float")
assert(math.huge == 1/0)
assert(-math.huge == -1/0)
assert(math.huge * 2 == math.huge)
assert(math.huge + 1 == math.huge)
assert(math.huge > 1.7976931348623157e308)
assert(-math.huge < -1.7976931348623157e308)
assert(math.huge - math.huge ~= math.huge - math.huge)
assert(math.max(1, math.huge) == math.huge)
assert(math.min(1, -math.huge) == -math.huge)
local n = 0
for i = 1, math.huge do
  n = i
  if i == 3 then break end
end
assert(n == 3)
local m = 0
for i = 3, -math.huge, -1 do
  m = m + 1
  if m == 2 then break end
end
assert(m == 2)
`); err != nil {
		t.Fatal(err)
	}
}
