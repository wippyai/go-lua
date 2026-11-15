package lua

import (
	"fmt"
	"strconv"
	"testing"
)

// maxStackOptions returns options for a state with the largest call stack.
func maxStackOptions(minimize bool) Options {
	return Options{CallStackSize: MaxCallStackSize, MinimizeStackMemory: minimize, RegistrySize: 1 << 16, RegistryMaxSize: 1 << 24, RegistryGrowStep: 1 << 16}
}

func TestFrameExtensionsSurviveDeepestStack(t *testing.T) {
	for _, minimize := range []bool{false, true} {
		t.Run(fmt.Sprintf("minimize=%v", minimize), func(t *testing.T) {
			frameExtensionsSurviveDeepestStack(t, maxStackOptions(minimize))
		})
	}
}

func frameExtensionsSurviveDeepestStack(t *testing.T, opts Options) {
	// Each level is a Lua frame and a pcall frame.
	const levels = MaxCallStackSize/2 - 8
	L := NewState(opts)
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

func TestCallStackOverflowIsAnErrorAtMaxCallStackSize(t *testing.T) {
	for _, minimize := range []bool{false, true} {
		L := NewState(maxStackOptions(minimize))
		err := L.DoString(`
local depth = 0
local function f() depth = depth + 1 return 1 + f() end
local ok, e = pcall(f)
assert(not ok and string.find(e, "stack overflow"), e)
assert(depth > 30000, depth)`)
		if err != nil {
			t.Fatalf("minimize=%v: %v", minimize, err)
		}
		L.Close()
	}
}
