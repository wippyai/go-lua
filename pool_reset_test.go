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
