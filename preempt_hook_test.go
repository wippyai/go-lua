package lua

import "testing"

func TestPreemptDispatchesHooksOncePerInstruction(t *testing.T) {
	src := `
local lines, counts = 0, 0
local function note(event)
	if event == "line" then lines = lines + 1 else counts = counts + 1 end
end
debug.sethook(note, "l", 1)
local s = 0
for i = 1, 20 do
	s = s + i
end
local function add(a, b) return a + b end
s = add(s, 1)
debug.sethook()
return lines, counts, s`
	L := NewState()
	defer L.Close()
	want, _ := runSliced(t, L, src, -1)
	for _, budget := range []int64{1, 2, 3} {
		got, preempts := runSliced(t, L, src, budget)
		if preempts == 0 {
			t.Fatalf("budget %d: expected preemption", budget)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("budget %d: results %v, want %v", budget, got, want)
			}
		}
	}
}
