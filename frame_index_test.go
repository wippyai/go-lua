package lua

import "testing"

func TestFrameExtensionsSurviveDeepStacks(t *testing.T) {
	const levels = 34000
	L := NewState(Options{CallStackSize: 2*levels + 100, RegistrySize: 1 << 16, RegistryMaxSize: 1 << 24, RegistryGrowStep: 1 << 16})
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
return f(` + "34000" + `)`)
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
