package lua

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

func collect() {
	for i := 0; i < 4; i++ {
		runtime.GC()
	}
}

func heapInUse() (inuse uint64, objects uint64) {
	collect()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse, m.HeapObjects
}

func requireNoFrameExt(t *testing.T, what string, th *LState) {
	t.Helper()
	if n := len(th.frameExt); n != 0 {
		t.Fatalf("%s: %d frame extensions remain with no active frames", what, n)
	}
}

// driveToCompletion resumes fn in th through every yield and preemption.
func driveToCompletion(t *testing.T, L, th *LState, fn *LFunction, budget int64) {
	t.Helper()
	for i := 0; i < 1_000_000; i++ {
		L.SetTickBudget(budget)
		st, _, err := L.Resume(th, fn)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		if st == ResumeOK {
			return
		}
	}
	t.Fatal("no progress")
}

var leakSources = map[string]string{
	"index_metamethod_yield": `
local t = setmetatable({}, {__index = function(_, k) coroutine.yield(k) return k end})
local s = 0
for i = 1, 5 do s = s + t[i] end
return s`,
	"arith_preempt": `
local mt = {__add = function(a, b) local s = 0 for i = 1, 50 do s = s + i end return s end}
local v = setmetatable({}, mt)
return v + 1`,
	"concat_yield": `
local v = setmetatable({}, {__concat = function(a, b) coroutine.yield(1) return "J" end})
return "a" .. v .. "b"`,
	"pcall_yield": `
local ok = pcall(function() coroutine.yield(1) return 1 end)
return ok`,
	"pcall_error_after_yield": `
local ok = pcall(function() coroutine.yield(1) error("x") end)
return ok`,
	"xpcall_yield": `
local ok = xpcall(function() coroutine.yield(1) error("x") end, function(e) return e end)
return ok`,
	"nested_resume_preempt": `
local co = coroutine.wrap(function() local s = 0 for i = 1, 100 do s = s + i end coroutine.yield(s) return 1 end)
co() co()
return 1`,
	"for_in_iterator_yield": `
local n = 0
for k in function() n = n + 1 coroutine.yield(n) if n < 4 then return n end end do end
return n`,
}

func TestFrameExtReleasedWhenFramesFinish(t *testing.T) {
	for name, src := range leakSources {
		for _, budget := range []int64{-1, 2} {
			t.Run(fmt.Sprintf("%s_budget%d", name, budget), func(t *testing.T) {
				L := NewState()
				defer L.Close()
				fn, err := L.LoadString(src)
				if err != nil {
					t.Fatal(err)
				}
				th, cancel := L.NewThread()
				defer cancel()
				driveToCompletion(t, L, th, fn, budget)
				requireNoFrameExt(t, "thread", th)
				requireNoFrameExt(t, "main", L)
			})
		}
	}
}

func TestFrameExtBoundedAcrossCycles(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(leakSources["index_metamethod_yield"])
	if err != nil {
		t.Fatal(err)
	}
	th, cancel := L.NewThread()
	defer cancel()
	for i := 0; i < 50; i++ {
		driveToCompletion(t, L, th, fn, 3)
		th.Dead = false
		th.stack.SetSp(0)
		th.currentFrame = nil
		requireNoFrameExt(t, "reused thread", th)
	}
}

func TestHoldsClearedAfterTeardown(t *testing.T) {
	setup := func(t *testing.T) (L, outer, inner *LState) {
		L = NewState()
		outer, _ = L.NewThread()
		outerFn, err := L.LoadString(`inner = coroutine.create(function() while true do end end) coroutine.resume(inner) return 1`)
		if err != nil {
			t.Fatal(err)
		}
		L.SetTickBudget(5)
		st, _, err := L.Resume(outer, outerFn)
		if err != nil || st != ResumePreempted {
			t.Fatalf("expected preemption, got %v %v", st, err)
		}
		inner = L.GetGlobal("inner").(*LState)
		if outer.holding != inner || inner.heldBy != outer {
			t.Fatal("hold not established")
		}
		return
	}
	t.Run("outer_closed", func(t *testing.T) {
		L, outer, inner := setup(t)
		defer L.Close()
		outer.Close()
		if inner.heldBy != nil {
			t.Fatal("inner still reserved after outer closed")
		}
	})
	t.Run("inner_closed", func(t *testing.T) {
		L, outer, inner := setup(t)
		defer L.Close()
		inner.Close()
		if outer.holding != nil {
			t.Fatal("outer still holds closed inner")
		}
	})
	t.Run("outer_killed", func(t *testing.T) {
		L, outer, inner := setup(t)
		defer L.Close()
		outer.kill()
		if inner.heldBy != nil || outer.holding != nil {
			t.Fatal("hold remains after kill")
		}
	})
}

func TestCoroutineCyclesDoNotGrowHeap(t *testing.T) {
	L := NewState()
	defer L.Close()
	script, err := L.LoadString(`
local t = setmetatable({}, {__index = function(_, k) coroutine.yield(k) return k end})
local ok = pcall(function() local x = t[1] coroutine.yield(x) error("e") end)
local co = coroutine.wrap(function() for i = 1, 20 do coroutine.yield(i) end end)
co() co()
local dead = coroutine.create(function() while true do end end)
return ok`)
	if err != nil {
		t.Fatal(err)
	}
	batch := func(n int) {
		for i := 0; i < n; i++ {
			th, cancel := L.NewThread()
			driveToCompletion(t, L, th, script, 4)
			cancel()
			th.Close()
		}
		L.SetTickBudget(-1)
	}
	batch(2000)
	baseInuse, baseObjs := heapInUse()
	batch(20000)
	inuse, objs := heapInUse()
	if objs > baseObjs+baseObjs/4+2000 {
		t.Fatalf("heap objects grew from %d to %d", baseObjs, objs)
	}
	if inuse > baseInuse+baseInuse/4+(1<<20) {
		t.Fatalf("heap in use grew from %d to %d", baseInuse, inuse)
	}
}

func TestSuspendedCoroutinesAreCollectable(t *testing.T) {
	cases := map[string]string{
		"preempted":      `while true do end`,
		"yielded_cont":   `local t = setmetatable({}, {__index = function(_, k) coroutine.yield(k) return k end}) return t.x`,
		"pcall_yield":    `pcall(function() coroutine.yield(1) end)`,
		"nested_preempt": `local inner = coroutine.create(function() while true do end end) coroutine.resume(inner)`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			done := make(chan struct{})
			func() {
				fn, err := L.LoadString("local keep = ... " + src)
				if err != nil {
					t.Fatal(err)
				}
				sentinel := L.NewTable()
				runtime.SetFinalizer(sentinel, func(*LTable) { close(done) })
				co, cancel := L.NewThread()
				defer cancel()
				L.SetTickBudget(5)
				if _, _, err := L.Resume(co, fn, sentinel); err != nil {
					t.Fatal(err)
				}
				L.SetTickBudget(-1)
			}()
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				collect()
				select {
				case <-done:
					return
				default:
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("suspended coroutine is not collectable")
		})
	}
}

func TestCountersRestoredAfterErrors(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("gopanic", L.NewFunction(func(L *LState) int { panic("boom") }))
	L.SetGlobal("goraise", L.NewFunction(func(L *LState) int { L.RaiseError("raised"); return 0 }))
	L.SetGlobal("gocall", L.NewFunction(func(L *LState) int {
		L.Push(L.Get(1))
		L.Call(0, 0)
		return 0
	}))
	srcs := []string{
		`pcall(gopanic)`,
		`pcall(goraise)`,
		`pcall(gocall, function() error("x") end)`,
		`pcall(gocall, goraise)`,
		`local co = coroutine.wrap(function() gocall(function() error("x") end) end) pcall(co)`,
		`local co = coroutine.create(function() error("x") end) coroutine.resume(co)`,
		`pcall(string.rep)`,
		`pcall(table.sort, {3, 2, 1}, function(a, b) error("cmp") end)`,
	}
	for _, src := range srcs {
		for _, budget := range []int64{-1, 1000} {
			fn, err := L.LoadString(src)
			if err != nil {
				t.Fatal(err)
			}
			th, cancel := L.NewThread()
			L.SetTickBudget(budget)
			func() {
				defer func() { _ = recover() }()
				_, _, _ = L.Resume(th, fn)
			}()
			cancel()
			L.SetTickBudget(-1)
			if L.G.nonYieldable != 0 || L.G.executing != 0 {
				t.Fatalf("%q budget %d: nonYieldable=%d executing=%d", src, budget, L.G.nonYieldable, L.G.executing)
			}
		}
	}
	for _, src := range srcs {
		func() {
			defer func() { _ = recover() }()
			_ = L.DoString(src)
		}()
		if L.G.nonYieldable != 0 || L.G.executing != 0 {
			t.Fatalf("%q on main: nonYieldable=%d executing=%d", src, L.G.nonYieldable, L.G.executing)
		}
	}
}
