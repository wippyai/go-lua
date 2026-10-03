package lua

import "testing"

func TestHeldChildReleasedBeforeResumeReportsDeadWrapper(t *testing.T) {
	L := NewState()
	defer L.Close()
	var child *LState
	L.SetGlobal("note", L.NewFunction(func(L *LState) int {
		child = L
		return 0
	}))
	fn, err := L.LoadString(`
local w = coroutine.wrap(function() note() while true do end end)
local ok = pcall(function() w() end)
return ok`)
	if err != nil {
		t.Fatal(err)
	}
	outer, cancel := L.NewThread()
	defer cancel()
	L.SetTickBudget(50)
	st, _, err := L.Resume(outer, fn)
	if err != nil || st != ResumePreempted {
		t.Fatalf("expected preemption, got %v %v", st, err)
	}
	if child == nil || !child.wrapped {
		t.Fatal("setup: wrapped child not captured")
	}
	child.Close()

	// The closed state returns to the pool and is reused by a plain thread.
	for i := 0; i < 8; i++ {
		_, c := L.NewThread()
		defer c()
	}
	L.SetTickBudget(-1)
	st, ret, err := L.Resume(outer, fn)
	if err != nil || st != ResumeOK {
		t.Fatalf("resume: %v %v %v", st, ret, err)
	}
	if len(ret) != 1 || ret[0] != LFalse {
		t.Fatalf("pcall around the dead wrapper returned %v, want false", ret)
	}
}
