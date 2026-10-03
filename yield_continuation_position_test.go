package lua

import (
	"strings"
	"testing"
	"time"
)

// A frame whose opcode awaits a suspended metamethod reports the position and
// the call name of that opcode, as it does when the metamethod ran to
// completion without suspending.
func TestSuspendedOpcodeFramePosition(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"go_tail_call_in_metamethod", `
local V = {}
V.__add = function(a, b) return setmetatable({}, 0) end
local acc = setmetatable({}, V)
local x = 1
return acc + acc`, ":6: bad argument #2 to (anonymous)"},
	}
	for _, tc := range cases {
		for _, budget := range []int64{-1, 1, 2, 3} {
			out, _, ok := diffRun(tc.src, nil, fixedBudget(budget), true, 1000, time.Time{})
			if !ok {
				t.Fatalf("%s: no progress", tc.name)
			}
			if !strings.Contains(out.err, tc.want) {
				t.Errorf("%s budget %d: error %q does not contain %q", tc.name, budget, out.err, tc.want)
			}
		}
	}
}
