package lua

import "testing"

func TestRootProtectedCallDeliversErrorAfterSuspension(t *testing.T) {
	for _, call := range []string{
		`pcall(function() %s error("boom") end)`,
		`xpcall(function() %s error("boom") end, function(e) return "H:" .. tostring(e) end)`,
	} {
		for _, mode := range []string{"yield", "preempt"} {
			for _, into := range []bool{false, true} {
				body, budget := "coroutine.yield(1)", int64(-1)
				if mode == "preempt" {
					body, budget = slowBody, 3
				}
				src := "return " + sprintfBody(call, body)
				L := NewState()
				fn, err := L.LoadString(src)
				if err != nil {
					t.Fatal(err)
				}
				co, cancel := L.NewThread()
				var ret []LValue
				for i := 0; ; i++ {
					if i > 1000 {
						t.Fatal("no progress")
					}
					L.SetTickBudget(budget)
					var st ResumeState
					var err error
					if into {
						st, ret, err = L.ResumeInto(co, fn, make([]LValue, 0, 4))
					} else {
						st, ret, err = L.Resume(co, fn)
					}
					if err != nil {
						t.Fatalf("%s %s into=%v: resume error %v", call, mode, into, err)
					}
					if st == ResumeOK {
						break
					}
				}
				if len(ret) != 2 || ret[0] != LFalse || ret[1] == LNil {
					t.Fatalf("%s %s into=%v: got %v", call, mode, into, ret)
				}
				if !co.Dead || co.Parent != nil || L.G.CurrentThread != L {
					t.Fatalf("%s %s: inconsistent state dead=%v parent=%v", call, mode, co.Dead, co.Parent)
				}
				cancel()
				L.Close()
			}
		}
	}
}

// heldPair returns an outer thread preempted inside coroutine.resume of inner.
func heldPair(t *testing.T) (L, outer, inner *LState) {
	t.Helper()
	L = NewState()
	fn, err := L.LoadString(`local s = 0 for i = 1, 1000 do s = s + i end return s`)
	if err != nil {
		t.Fatal(err)
	}
	L.SetGlobal("innerfn", fn)
	if err := L.DoString(`inner_co = coroutine.create(innerfn)`); err != nil {
		t.Fatal(err)
	}
	inner = L.GetGlobal("inner_co").(*LState)
	outerFn, err := L.LoadString(`return coroutine.resume(inner_co)`)
	if err != nil {
		t.Fatal(err)
	}
	outer, _ = L.NewThread()
	L.SetTickBudget(5)
	st, _, err := L.Resume(outer, outerFn)
	if err != nil || st != ResumePreempted {
		t.Fatalf("expected preemption, got %v %v", st, err)
	}
	L.SetTickBudget(-1)
	if !inner.isHeld() || outer.holding != inner {
		t.Fatal("inner is not held by outer")
	}
	return L, outer, inner
}

func TestHoldReleasedWhenResumerIsKilled(t *testing.T) {
	L, outer, inner := heldPair(t)
	defer L.Close()
	outer.kill()
	if inner.isHeld() || outer.holding != nil {
		t.Fatal("hold survives resumer kill")
	}
	if msg := L.resumeRejection(inner, 0); msg != "" {
		t.Fatalf("inner rejected: %s", msg)
	}
}

func TestHoldReleasedWhenResumerIsClosed(t *testing.T) {
	L, outer, inner := heldPair(t)
	defer L.Close()
	outer.Close()
	if inner.isHeld() {
		t.Fatal("hold survives resumer close")
	}
}

func TestHoldReleasedWhenChildIsClosedOrKilled(t *testing.T) {
	for _, closeIt := range []bool{true, false} {
		L, outer, inner := heldPair(t)
		if closeIt {
			inner.Close()
		} else {
			inner.kill()
		}
		if inner.heldBy != nil || outer.holding != nil {
			t.Fatalf("closeIt=%v: hold survives child teardown", closeIt)
		}
		L.Close()
	}
}

// Resuming an owner whose held child was torn down reports a dead coroutine
// and never touches the child.
func TestOwnerResumeAfterHeldChildTeardown(t *testing.T) {
	teardowns := map[string]func(L, outer, inner *LState){
		"kill":  func(_, _, inner *LState) { inner.kill() },
		"close": func(_, _, inner *LState) { inner.Close() },
		"pool_reuse": func(L, _, inner *LState) {
			inner.Close()
			reused, _ := L.NewThread()
			reused.SetTop(0)
		},
	}
	for name, teardown := range teardowns {
		t.Run(name, func(t *testing.T) {
			L, outer, inner := heldPair(t)
			defer L.Close()
			teardown(L, outer, inner)
			outerFn, err := L.LoadString(`return coroutine.resume(inner_co)`)
			if err != nil {
				t.Fatal(err)
			}
			st, ret, err := L.Resume(outer, outerFn)
			if err != nil || st != ResumeOK {
				t.Fatalf("expected completion, got %v %v %v", st, ret, err)
			}
			if len(ret) != 2 || ret[0] != LFalse {
				t.Fatalf("expected failed resume, got %v", ret)
			}
		})
	}
}

func TestHoldReleasedWhenUnpoolableOwnerIsClosed(t *testing.T) {
	L, outer, inner := heldPair(t)
	defer L.Close()
	outer.reg.resize(outer.Options.RegistrySize + outer.Options.RegistryGrowStep + 1024)
	outer.Close()
	if inner.isHeld() {
		t.Fatal("hold survives close of an unpoolable owner")
	}
}

func TestSetTickBudgetRejectedInsideDirectChildCall(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("setbudget", L.NewFunction(func(*LState) int {
		L.SetTickBudget(0)
		return 0
	}))
	for _, viaPCall := range []bool{false, true} {
		child, cancel := L.NewThread()
		fn, err := L.LoadString(`setbudget()`)
		if err != nil {
			t.Fatal(err)
		}
		child.Push(fn)
		if viaPCall {
			if err := child.PCall(0, 0, nil); err == nil {
				t.Fatal("expected the budget change to be rejected")
			}
		} else {
			func() {
				defer func() {
					if recover() == nil {
						t.Error("expected the budget change to be rejected")
					}
				}()
				child.Call(0, 0)
			}()
		}
		cancel()
		if L.TickBudget() >= 0 {
			t.Fatalf("viaPCall=%v: budget changed to %d", viaPCall, L.TickBudget())
		}
	}
}
