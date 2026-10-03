package lua

import "testing"

func TestResetLStateClearsEveryReference(t *testing.T) {
	L := NewState()
	defer L.Close()
	th, cancel := L.NewThread()
	defer cancel()
	other, cancel2 := L.NewThread()
	defer cancel2()
	fn, err := L.LoadString(`local t = setmetatable({}, {__index = function(_, k) coroutine.yield(k) return k end}) return t.x`)
	if err != nil {
		t.Fatal(err)
	}
	if st, _, err := L.Resume(th, fn); err != nil || st != ResumeYield {
		t.Fatalf("setup yield: %v %v", st, err)
	}
	if len(th.frameExt) == 0 {
		t.Fatal("setup: expected frame extensions")
	}
	th.holding, other.heldBy = other, th
	th.yieldCallRB = 7
	th.Dead = true

	resetLState(th)

	switch {
	case th.G != nil, th.Parent != nil, th.Env != nil, th.currentFrame != nil, th.uvcache != nil:
		t.Fatal("pooled state retains a state reference")
	case th.frameExt != nil:
		t.Fatal("pooled state retains frame extensions")
	case th.holding != nil, th.heldBy != nil:
		t.Fatal("pooled state retains hold references")
	case th.ctx != nil, th.ctxDone != nil, th.ctxCancelFn != nil:
		t.Fatal("pooled state retains a context")
	case th.yieldState != yieldNone, th.yieldCallRB != 0:
		t.Fatal("pooled state retains yield state")
	}
	for i := range th.reg.array {
		if th.reg.array[i] != LNil && th.reg.array[i] != nil {
			t.Fatalf("pooled registry slot %d retains %v", i, th.reg.array[i])
		}
	}
}

func TestPooledStateFramesDoNotRetainClosures(t *testing.T) {
	L := NewState()
	defer L.Close()
	th, cancel := L.NewThread()
	defer cancel()
	fn, err := L.LoadString(`local t = setmetatable({}, {__index = function(_, k) coroutine.yield(k) return k end}) return t.x`)
	if err != nil {
		t.Fatal(err)
	}
	if st, _, err := L.Resume(th, fn); err != nil || st != ResumeYield {
		t.Fatalf("setup yield: %v %v", st, err)
	}
	depth := th.stack.Sp()
	if depth == 0 {
		t.Fatal("setup: no frames")
	}
	resetLState(th)
	for i := 0; i < depth; i++ {
		if f := th.stack.At(i); f.Fn != nil || f.GoFunc != nil {
			t.Fatalf("pooled frame %d retains a function", i)
		}
	}
}

const recurseSource = `local function f(n) if n == 0 then return 0 end return 1 + f(n - 1) end return f(...)`

func TestPooledStateHonorsCallStackSize(t *testing.T) {
	for _, minimize := range []bool{false, true} {
		NewState(Options{CallStackSize: 16, MinimizeStackMemory: minimize}).Close()
		L := NewState(Options{CallStackSize: 1024, MinimizeStackMemory: minimize})
		fn, err := L.LoadString(recurseSource)
		if err != nil {
			t.Fatal(err)
		}
		L.Push(fn)
		L.Push(LNumber(500))
		if err := L.PCall(1, 1, nil); err != nil {
			t.Fatalf("minimize=%v: %v", minimize, err)
		}
		L.Close()
	}
}

func TestPooledThreadHonorsCallStackSize(t *testing.T) {
	for _, minimize := range []bool{false, true} {
		L := NewState(Options{CallStackSize: 1024, MinimizeStackMemory: minimize})
		NewState(Options{CallStackSize: 16, MinimizeStackMemory: minimize}).Close()
		fn, err := L.LoadString(recurseSource)
		if err != nil {
			t.Fatal(err)
		}
		th, cancel := L.NewThread()
		st, ret, err := L.Resume(th, fn, LNumber(500))
		cancel()
		if err != nil || st != ResumeOK {
			t.Fatalf("minimize=%v: %v %v %v", minimize, st, ret, err)
		}
		L.Close()
	}
}

func TestPooledThreadHonorsRegistryLimits(t *testing.T) {
	L := NewState(Options{RegistrySize: 256, RegistryMaxSize: 4096, RegistryGrowStep: 64})
	defer L.Close()
	NewState(Options{RegistrySize: 256, RegistryMaxSize: 1 << 20, RegistryGrowStep: 1024}).Close()
	th, cancel := L.NewThread()
	defer cancel()
	if th.reg.maxSize != 4096 || th.reg.growBy != 64 {
		t.Fatalf("thread registry limits %d/%d, want 4096/64", th.reg.maxSize, th.reg.growBy)
	}
}

func TestPooledStateRegistryRespectsMaxSize(t *testing.T) {
	NewState(Options{RegistrySize: 4096, RegistryMaxSize: 4096}).Close()
	L := NewState(Options{RegistrySize: 256, RegistryMaxSize: 512})
	defer L.Close()
	if n := cap(L.reg.array); n > 512 {
		t.Fatalf("registry holds %d slots, limit is 512", n)
	}
}
