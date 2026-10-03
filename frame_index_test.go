package lua

import (
	"strconv"
	"testing"
)

func TestFrameExtensionsSurviveDeepestStack(t *testing.T) {
	// Each level is a Lua frame and a pcall frame.
	const levels = MaxCallStackSize/2 - 8
	L := NewState(Options{CallStackSize: MaxCallStackSize, RegistrySize: 1 << 16, RegistryMaxSize: 1 << 24, RegistryGrowStep: 1 << 16})
	defer L.Close()
	fn, err := L.LoadString(`
local function f(n)
	if n == 0 then
		coroutine.yield(1)
		return 0
	end
	local ok, v = pcall(f, n - 1)
	if not ok then error(v, 0) end
	return v + 1
end
return f(` + strconv.Itoa(levels) + `)`)
	if err != nil {
		t.Fatal(err)
	}
	th, cancel := L.NewThread()
	defer cancel()
	st, _, err := L.Resume(th, fn)
	if err != nil || st != ResumeYield {
		t.Fatalf("first resume: %v %v", st, err)
	}
	st, ret, err := L.Resume(th, fn, LNumber(1))
	if err != nil || st != ResumeOK {
		t.Fatalf("second resume: %v %v %v", st, ret, err)
	}
	expectNumbers(t, ret, levels)
}

func TestCallStackSizeBeyondFrameIndexRejected(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a call stack larger than frame indices can address is refused")
		}
	}()
	NewState(Options{CallStackSize: MaxCallStackSize + 1})
}
