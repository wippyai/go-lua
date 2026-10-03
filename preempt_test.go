package lua

import (
	"context"
	"strings"
	"testing"
)

// runSliced resumes src in a fresh thread with the given tick budget per
// resume until it finishes, and returns its results and preemption count.
func runSliced(t *testing.T, L *LState, src string, budget int64) ([]LValue, int) {
	t.Helper()
	fn, err := L.LoadString(src)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	co, cancel := L.NewThread()
	defer cancel()

	preempts := 0
	for {
		L.SetTickBudget(budget)
		st, ret, err := L.Resume(co, fn)
		if err != nil {
			t.Fatalf("resume after %d preemptions: %v", preempts, err)
		}
		switch st {
		case ResumePreempted:
			preempts++
			if preempts > 1_000_000 {
				t.Fatalf("no progress after %d preemptions", preempts)
			}
		case ResumeOK:
			return ret, preempts
		default:
			t.Fatalf("unexpected resume state %v (%v)", st, ret)
		}
	}
}

func expectNumbers(t *testing.T, got []LValue, want ...LNumber) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d results %v, got %v", len(want), want, got)
	}
	for i := range want {
		if LVAsNumber(got[i]) != want[i] {
			t.Fatalf("result %d: expected %v, got %v", i, want[i], got[i])
		}
	}
}

func TestPreemptSafepoints(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want LNumber
	}{
		{"numeric_for", `local s = 0 for i = 1, 10000 do s = s + i end return s`, 50005000},
		{"while_backjump", `local s, i = 0, 0 while i < 10000 do i = i + 1 s = s + i end return s`, 50005000},
		{"repeat_until", `local s, i = 0, 0 repeat i = i + 1 s = s + i until i >= 10000 return s`, 50005000},
		{"generic_for_lua_iterator", `
local function iter(n, i) if i < n then return i + 1 end end
local s = 0
for i in iter, 10000, 0 do s = s + i end
return s`, 50005000},
		{"recursion", `
local function sum(n) if n == 0 then return 0 end return n + sum(n - 1) end
return sum(100)`, 5050},
		{"tail_recursion", `
local function sum(n, acc) if n == 0 then return acc end return sum(n - 1, acc + n) end
return sum(10000, 0)`, 50005000},
		{"nested_loops_with_calls", `
local function f(x) return x + 1 end
local s = 0
for i = 1, 100 do for j = 1, 100 do s = f(s) end end
return s`, 10000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			ret, preempts := runSliced(t, L, tc.src, 7)
			expectNumbers(t, ret, tc.want)
			if preempts == 0 {
				t.Fatal("expected preemption")
			}
		})
	}
}

func TestPreemptAcrossYieldableBoundaries(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want LNumber
	}{
		{"pcall", `
local ok, v = pcall(function() local s = 0 for i = 1, 1000 do s = s + i end return s end)
assert(ok)
return v`, 500500},
		{"nested_pcall", `
local ok, v = pcall(pcall, function() local s = 0 for i = 1, 1000 do s = s + i end return s end)
assert(ok)
return 1`, 1},
		{"xpcall", `
local ok, v = xpcall(function() local s = 0 for i = 1, 1000 do s = s + i end return s end, function(e) return e end)
assert(ok)
return v`, 500500},
		{"pcall_error_after_preempt", `
local ok, err = pcall(function() local s = 0 for i = 1, 1000 do s = s + i end error("boom") end)
assert(not ok and string.find(err, "boom"))
return 1`, 1},
		{"index_metamethod", `
local t = setmetatable({}, {__index = function(_, k) local s = 0 for i = 1, k do s = s + i end return s end})
return t[1000]`, 500500},
		{"newindex_metamethod", `
local store = {}
local t = setmetatable({}, {__newindex = function(_, k, v) local s = 0 for i = 1, v do s = s + i end store[k] = s end})
t.x = 1000
return store.x`, 500500},
		{"nested_index_metamethods", `
local inner = setmetatable({}, {__index = function(_, k) local s = 0 for i = 1, k do s = s + i end return s end})
local outer = setmetatable({}, {__index = function(_, k) local v = inner[k] for i = 1, 100 do v = v + 0 end return v + 1 end})
return outer[1000]`, 500501},
		{"arith_metamethod", `
local mt = {__add = function(a, b) local s = 0 for i = 1, b do s = s + i end return s end}
local v = setmetatable({}, mt)
return v + 1000`, 500500},
		{"compare_metamethod", `
local mt = {__lt = function(a, b) local s = 0 for i = 1, 1000 do s = s + i end return s == 500500 end}
local a, b = setmetatable({}, mt), setmetatable({}, mt)
if a < b then return 1 end
return 0`, 1},
		{"concat_metamethod", `
local mt = {__concat = function(a, b) local s = 0 for i = 1, 1000 do s = s + i end return s end}
local v = setmetatable({}, mt)
return v .. "x"`, 500500},
		{"len_and_unm_metamethods", `
local mt = {
  __len = function() local s = 0 for i = 1, 1000 do s = s + i end return s end,
  __unm = function() local s = 0 for i = 1, 1000 do s = s + i end return s end,
}
local v = setmetatable({}, mt)
return #v + -v`, 1001000},
		{"self_call_through_index", `
local obj = setmetatable({}, {__index = function(_, k)
  local s = 0 for i = 1, 100 do s = s + i end
  return function(self, n) local r = 0 for i = 1, n do r = r + i end return r + s end
end})
return obj:sum(1000)`, 505550},
		{"coroutine_resume", `
local co = coroutine.create(function(n) local s = 0 for i = 1, n do s = s + i end return s end)
local ok, v = coroutine.resume(co, 1000)
assert(ok and coroutine.status(co) == "dead")
return v`, 500500},
		{"coroutine_with_user_yields", `
local co = coroutine.create(function()
  for k = 1, 3 do local s = 0 for i = 1, 1000 do s = s + i end coroutine.yield(s + k) end
  return 0
end)
local total = 0
while true do
  local ok, v = coroutine.resume(co)
  assert(ok)
  if coroutine.status(co) == "dead" then break end
  total = total + v
end
return total`, 1501506},
		{"coroutine_wrap", `
local gen = coroutine.wrap(function() local s = 0 for i = 1, 1000 do s = s + i end coroutine.yield(s) return 1 end)
return gen() + gen()`, 500501},
		{"coroutine_nested_two_levels", `
local outer = coroutine.wrap(function()
  local inner = coroutine.wrap(function() local s = 0 for i = 1, 1000 do s = s + i end return s end)
  return inner() + 1
end)
return outer()`, 500501},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			ret, preempts := runSliced(t, L, tc.src, 5)
			expectNumbers(t, ret, tc.want)
			if preempts == 0 {
				t.Fatal("expected preemption")
			}
		})
	}
}

// Lua invoked by Go code without a continuation runs under a Go frame that
// cannot be suspended; it runs to completion and preemption happens after.
func TestPreemptSuppressedUnderGoCallers(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want LNumber
	}{
		{"table_sort_comparator", `
local t = {}
for i = 1, 50 do t[i] = (i * 37) % 50 end
table.sort(t, function(a, b) local s = 0 for i = 1, 20 do s = s + i end return a < b end)
for i = 2, #t do assert(t[i - 1] <= t[i]) end
return t[50]`, 49},
		{"gsub_callback", `
local s = string.gsub("aaaa", "a", function(c) local n = 0 for i = 1, 100 do n = n + 1 end return tostring(n) end)
return #s`, 12},
		{"xpcall_handler", `
local ok, v = xpcall(function() error("x") end, function(e) local s = 0 for i = 1, 1000 do s = s + i end return s end)
assert(not ok)
return v`, 500500},
		{"go_function_callback", `
return gocall(function(n) local s = 0 for i = 1, n do s = s + i end return s end, 1000)`, 500500},
		{"tostring_metamethod", `
local v = setmetatable({}, {__tostring = function() local s = 0 for i = 1, 1000 do s = s + i end return tostring(s) end})
return tonumber(tostring(v))`, 500500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetGlobal("gocall", L.NewFunction(func(L *LState) int {
				L.Push(L.CheckFunction(1))
				L.Push(L.Get(2))
				L.Call(1, 1)
				return 1
			}))
			ret, _ := runSliced(t, L, tc.src, 3)
			expectNumbers(t, ret, tc.want)
		})
	}
}

// A thread resumed by Go code that is itself running inside the VM cannot
// surface a preemption through that Go frame, so it is not preempted.
func TestPreemptSuppressedForResumeFromGoFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("gorun", L.NewFunction(func(L *LState) int {
		fn := L.CheckFunction(1)
		co, cancel := L.NewThread()
		defer cancel()
		st, ret, err := L.Resume(co, fn)
		if err != nil {
			L.RaiseError("%v", err)
		}
		if st != ResumeOK {
			L.RaiseError("inner thread state %v", st)
		}
		L.Push(ret[0])
		return 1
	}))
	ret, _ := runSliced(t, L, `return gorun(function() local s = 0 for i = 1, 1000 do s = s + i end return s end)`, 3)
	expectNumbers(t, ret, 500500)
}

func TestPreemptDisabledByDefault(t *testing.T) {
	L := NewState()
	defer L.Close()
	if L.TickBudget() >= 0 {
		t.Fatalf("expected unlimited default budget, got %d", L.TickBudget())
	}
	fn, err := L.LoadString(`local s = 0 for i = 1, 100000 do s = s + i end return s`)
	if err != nil {
		t.Fatal(err)
	}
	co, cancel := L.NewThread()
	defer cancel()
	st, ret, err := L.Resume(co, fn)
	if err != nil || st != ResumeOK {
		t.Fatalf("expected ResumeOK, got %v %v %v", st, ret, err)
	}
	expectNumbers(t, ret, 5000050000)
}

func TestPreemptNotAppliedToMainThread(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetTickBudget(1)
	if err := L.DoString(`result = 0 for i = 1, 1000 do result = result + i end`); err != nil {
		t.Fatal(err)
	}
	expectNumbers(t, []LValue{L.GetGlobal("result")}, 500500)
}

func TestPreemptBudgetAccounting(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`local s = 0 for i = 1, 1000 do s = s + i end return s`)
	if err != nil {
		t.Fatal(err)
	}
	co, cancel := L.NewThread()
	defer cancel()

	L.SetTickBudget(10)
	st, _, err := L.Resume(co, fn)
	if err != nil || st != ResumePreempted {
		t.Fatalf("expected ResumePreempted, got %v %v", st, err)
	}
	if L.TickBudget() != 0 {
		t.Fatalf("expected exhausted budget, got %d", L.TickBudget())
	}
	if got := L.Status(co); got != "suspended" {
		t.Fatalf("expected suspended thread, got %s", got)
	}

	L.SetTickBudget(-1)
	st, ret, err := L.Resume(co, fn)
	if err != nil || st != ResumeOK {
		t.Fatalf("expected ResumeOK, got %v %v", st, err)
	}
	expectNumbers(t, ret, 500500)
}

func TestPreemptedThreadRejectsResumeValues(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`local s = 0 for i = 1, 1000 do s = s + i end return s`)
	if err != nil {
		t.Fatal(err)
	}
	co, cancel := L.NewThread()
	defer cancel()

	L.SetTickBudget(10)
	if st, _, err := L.Resume(co, fn); err != nil || st != ResumePreempted {
		t.Fatalf("expected ResumePreempted, got %v %v", st, err)
	}
	L.SetTickBudget(-1)
	st, _, err := L.Resume(co, fn, LNumber(1))
	if err == nil || st != ResumeError || !strings.Contains(err.Error(), "preempted") {
		t.Fatalf("expected preempted-resume error, got %v %v", st, err)
	}
}

func TestPreemptResumeIntoReusesBuffer(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`local s = 0 for i = 1, 1000 do s = s + i end return s, "done"`)
	if err != nil {
		t.Fatal(err)
	}
	co, cancel := L.NewThread()
	defer cancel()

	buf := make([]LValue, 0, 4)
	preempts := 0
	for {
		L.SetTickBudget(16)
		st, ret, err := L.ResumeInto(co, fn, buf)
		if err != nil {
			t.Fatal(err)
		}
		if st == ResumePreempted {
			if len(ret) != 0 {
				t.Fatalf("preemption carries no values, got %v", ret)
			}
			preempts++
			continue
		}
		if st != ResumeOK || len(ret) != 2 || LVAsNumber(ret[0]) != 500500 || ret[1] != LString("done") {
			t.Fatalf("unexpected completion %v %v", st, ret)
		}
		break
	}
	if preempts == 0 {
		t.Fatal("expected preemption")
	}
}

func TestPreemptStillHonorsContextCancellation(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`while true do end`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	co := L.NewThreadWithContext(ctx)

	for i := 0; i < 3; i++ {
		L.SetTickBudget(100)
		st, _, err := L.Resume(co, fn)
		if err != nil || st != ResumePreempted {
			t.Fatalf("expected ResumePreempted, got %v %v", st, err)
		}
	}
	cancel()
	L.SetTickBudget(-1)
	st, _, err := L.Resume(co, fn)
	if err == nil || st != ResumeError {
		t.Fatalf("expected cancellation error, got %v %v", st, err)
	}
}

// A system yield and a preemption interleave in the same thread.
func TestPreemptInterleavesWithSystemYield(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("yield", L.NewFunction(yieldingGoFunc))
	fn, err := L.LoadString(`
local s = 0
for k = 1, 3 do
  for i = 1, 1000 do s = s + i end
  s = s + yield(k)
end
return s`)
	if err != nil {
		t.Fatal(err)
	}
	co, cancel := L.NewThread()
	defer cancel()

	var args []LValue
	yields, preempts := 0, 0
	for {
		L.SetTickBudget(50)
		st, ret, err := L.Resume(co, fn, args...)
		args = nil
		if err != nil {
			t.Fatal(err)
		}
		switch st {
		case ResumePreempted:
			preempts++
		case ResumeYield:
			yields++
			args = []LValue{LNumber(1)}
		case ResumeOK:
			expectNumbers(t, ret, 3*500500+3)
			if yields != 3 || preempts == 0 {
				t.Fatalf("expected 3 yields and some preemptions, got %d/%d", yields, preempts)
			}
			return
		}
	}
}

// A Go frame suspended while preemption was disabled resumes under an enabled
// budget; the frames it enters afterwards are counted and balanced.
func TestPreemptEnabledAfterYieldInsideGoFrame(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`
		local ok, v = pcall(function()
			coroutine.yield(1)
			local s = 0
			for i = 1, 1000 do s = s + i end
			return s
		end)
		return ok, v`)
	if err != nil {
		t.Fatal(err)
	}
	co, cancel := L.NewThread()
	defer cancel()
	st, _, err := L.Resume(co, fn)
	if err != nil || st != ResumeYield {
		t.Fatalf("expected yield, got %v %v", st, err)
	}
	preempts := 0
	L.SetTickBudget(3)
	for {
		st, ret, err := L.Resume(co, fn)
		if err != nil {
			t.Fatal(err)
		}
		if st == ResumePreempted {
			preempts++
			L.SetTickBudget(3)
			continue
		}
		if st != ResumeOK || ret[0] != LTrue {
			t.Fatalf("unexpected result %v %v", st, ret)
		}
		expectNumbers(t, ret[1:], 500500)
		break
	}
	if preempts == 0 {
		t.Fatal("expected preemption after enabling the budget")
	}
	if L.G.nonYieldable != 0 {
		t.Fatalf("nonYieldable = %d", L.G.nonYieldable)
	}
}

// Preemption suspended inside a yieldable Go frame resumes with preemption
// disabled; no counter is left behind.
func TestPreemptDisabledAfterPreemptInsideGoFrame(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`
		local ok, v = pcall(function()
			local s = 0
			for i = 1, 1000 do s = s + i end
			return s
		end)
		return ok, v`)
	if err != nil {
		t.Fatal(err)
	}
	co, cancel := L.NewThread()
	defer cancel()
	L.SetTickBudget(3)
	st, _, err := L.Resume(co, fn)
	if err != nil || st != ResumePreempted {
		t.Fatalf("expected preemption, got %v %v", st, err)
	}
	L.SetTickBudget(-1)
	st, ret, err := L.Resume(co, fn)
	if err != nil || st != ResumeOK || ret[0] != LTrue {
		t.Fatalf("unexpected result %v %v %v", st, ret, err)
	}
	expectNumbers(t, ret[1:], 500500)
	if L.G.nonYieldable != 0 {
		t.Fatalf("nonYieldable = %d", L.G.nonYieldable)
	}
}

// A thread held by its resumer's pending continuation cannot be resumed by
// anyone else, and the continuation still completes it.
func TestPreemptedChildIsReservedByItsResumer(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		inner = coroutine.create(function()
			local s = 0
			for i = 1, 1000 do s = s + i end
			return s
		end)`); err != nil {
		t.Fatal(err)
	}
	outerFn, err := L.LoadString(`return coroutine.resume(inner)`)
	if err != nil {
		t.Fatal(err)
	}
	outer, cancelOuter := L.NewThread()
	defer cancelOuter()
	L.SetTickBudget(5)
	st, _, err := L.Resume(outer, outerFn)
	if err != nil || st != ResumePreempted {
		t.Fatalf("expected preemption, got %v %v", st, err)
	}

	L.SetTickBudget(-1)
	ret, _ := runSliced(t, L, `return coroutine.resume(inner)`, -1)
	if len(ret) != 2 || ret[0] != LFalse {
		t.Fatalf("sibling resume of a held thread must fail, got %v", ret)
	}
	if got := mustDoString(t, L, `return coroutine.status(inner)`); got != "normal" {
		t.Fatalf("held thread status = %q", got)
	}

	st, ret, err = L.Resume(outer, outerFn)
	if err != nil || st != ResumeOK {
		t.Fatalf("expected completion, got %v %v %v", st, ret, err)
	}
	if len(ret) != 2 || ret[0] != LTrue || LVAsNumber(ret[1]) != 500500 {
		t.Fatalf("unexpected result %v", ret)
	}
}

// Values cannot be passed to a preempted thread from Lua either.
func TestLuaResumeRejectsValuesForPreemptedThread(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`local s = 0 for i = 1, 1000 do s = s + i end return s`)
	if err != nil {
		t.Fatal(err)
	}
	th, cancel := L.NewThread()
	defer cancel()
	L.SetGlobal("th", th)
	L.SetTickBudget(5)
	st, _, err := L.Resume(th, fn)
	if err != nil || st != ResumePreempted {
		t.Fatalf("expected preemption, got %v %v", st, err)
	}
	L.SetTickBudget(-1)
	ret, _ := runSliced(t, L, `return coroutine.resume(th, 1)`, -1)
	if len(ret) != 2 || ret[0] != LFalse {
		t.Fatalf("expected rejection, got %v", ret)
	}
	ret, _ = runSliced(t, L, `return coroutine.resume(th)`, -1)
	if len(ret) != 2 || ret[0] != LTrue || LVAsNumber(ret[1]) != 500500 {
		t.Fatalf("expected completion, got %v", ret)
	}
}

func mustDoString(t *testing.T, L *LState, src string) string {
	t.Helper()
	ret, _ := runSliced(t, L, src, -1)
	if len(ret) != 1 {
		t.Fatalf("unexpected results %v", ret)
	}
	return ret[0].String()
}

// Preempted resumes return no results and allocate nothing.
func TestPreemptedResumeDoesNotAllocate(t *testing.T) {
	L := NewState()
	defer L.Close()
	fn, err := L.LoadString(`while true do end`)
	if err != nil {
		t.Fatal(err)
	}
	th, cancel := L.NewThread()
	defer cancel()
	L.SetTickBudget(0)
	if st, _, err := L.ResumeInto(th, fn, nil); err != nil || st != ResumePreempted {
		t.Fatalf("expected preemption, got %v %v", st, err)
	}
	allocs := testing.AllocsPerRun(100, func() {
		L.SetTickBudget(0)
		st, ret, err := L.ResumeInto(th, fn, nil)
		if err != nil || st != ResumePreempted || len(ret) != 0 {
			t.Fatalf("unexpected resume %v %v %v", st, ret, err)
		}
	})
	if allocs != 0 {
		t.Fatalf("expected no allocations, got %v", allocs)
	}
}

func TestSetTickBudgetRejectedWhileLuaRuns(t *testing.T) {
	cases := map[string]bool{"main": false, "coroutine": true}
	for name, inCoroutine := range cases {
		t.Run(name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetGlobal("setbudget", L.NewFunction(func(L *LState) int {
				L.SetTickBudget(0)
				return 0
			}))
			src := `return pcall(setbudget)`
			var ret []LValue
			if inCoroutine {
				ret, _ = runSliced(t, L, src, -1)
			} else {
				if err := L.DoString(`result = {pcall(setbudget)}`); err != nil {
					t.Fatal(err)
				}
				ret = []LValue{L.GetGlobal("result").(*LTable).RawGetInt(1)}
			}
			if ret[0] != LFalse {
				t.Fatalf("expected rejection, got %v", ret)
			}
			if L.TickBudget() >= 0 {
				t.Fatalf("budget changed to %d", L.TickBudget())
			}
		})
	}
}

func TestSetTickBudgetBetweenResumes(t *testing.T) {
	L := NewState()
	defer L.Close()
	ret, preempts := runSliced(t, L, `local s = 0 for i = 1, 100 do s = s + i end return s`, 5)
	if preempts == 0 {
		t.Fatal("expected preemption")
	}
	expectNumbers(t, ret, 5050)
	L.SetTickBudget(-1)
	if L.TickBudget() != -1 {
		t.Fatal("budget not reset")
	}
}
