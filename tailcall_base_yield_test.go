package lua

import (
	"context"
	"strings"
	"testing"
)

func TestTailCallYieldFromCoroutineBase(t *testing.T) {
	L := NewState()
	defer L.Close()

	calls := 0
	L.SetGlobal("yield_sentinel", L.NewFunction(func(L *LState) int {
		calls++
		L.Push(LString("sentinel-token"))
		return -1
	}))
	L.SetGlobal("yield_api", L.NewFunction(func(L *LState) int {
		calls++
		return L.Yield(LString("api-token"))
	}))
	L.SetGlobal("yield_stateless", LGoFunc(func(L *LState) int {
		calls++
		return L.Yield(LString("stateless-token"))
	}))
	if err := L.DoString(`
		tail_sentinel = function() return yield_sentinel("arg1", "arg2") end
		tail_api = function() return yield_api("arg") end
		tail_stateless = function() return yield_stateless("arg") end
		tail_user = function() return coroutine.yield("user-token") end
		tail_method = function()
			local receiver = { call = yield_api }
			return receiver:call("arg")
		end
		tail_callable = function()
			local callable = setmetatable({}, { __call = yield_api })
			return callable("arg")
		end
	`); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "tail_sentinel", token: "sentinel-token"},
		{name: "tail_api", token: "api-token"},
		{name: "tail_stateless", token: "stateless-token"},
		{name: "tail_user", token: "user-token"},
		{name: "tail_method", token: "api-token"},
		{name: "tail_callable", token: "api-token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := L.GetGlobal(test.name).(*LFunction)
			co := L.NewThreadWithContext(context.Background())

			state, values, err := L.Resume(co, fn)
			if err != nil {
				t.Fatalf("first resume: %v", err)
			}
			if state != ResumeYield {
				t.Fatalf("first resume state = %v, want ResumeYield; values=%v", state, values)
			}
			if len(values) == 0 || values[len(values)-1].String() != test.token {
				t.Fatalf("first resume values = %v, want final yield token %q", values, test.token)
			}
			if co.stack.IsEmpty() || co.currentFrame == nil {
				t.Fatal("reported yield did not retain a resumable frame")
			}

			state, values, err = L.Resume(co, fn, LString("result"))
			if err != nil {
				t.Fatalf("second resume: %v", err)
			}
			if state != ResumeOK || len(values) != 1 || values[0] != LString("result") {
				t.Fatalf("second resume = (%v, %v), want (ResumeOK, [result])", state, values)
			}
			assertNoContinuationMetadata(t, co)
		})
	}
	const wantCalls = 5 // all cases except Lua's coroutine.yield
	if calls != wantCalls {
		t.Fatalf("yielding Go functions invoked %d times, want %d", calls, wantCalls)
	}
}

func TestTailCallYieldFromCoroutineBaseResumeInto(t *testing.T) {
	L := NewState()
	defer L.Close()

	calls := 0
	L.SetGlobal("yield_api", L.NewFunction(func(L *LState) int {
		calls++
		return L.Yield(LString("token"))
	}))
	if err := L.DoString(`tail = function() return yield_api("arg") end`); err != nil {
		t.Fatal(err)
	}

	fn := L.GetGlobal("tail").(*LFunction)
	co := L.NewThreadWithContext(context.Background())
	buf := make([]LValue, 0, 2)

	state, values, err := L.ResumeInto(co, fn, buf)
	if err != nil {
		t.Fatalf("first resume: %v", err)
	}
	if state != ResumeYield || len(values) != 1 || values[0] != LString("token") {
		t.Fatalf("first resume = (%v, %v), want (ResumeYield, [token])", state, values)
	}
	state, values, err = L.ResumeInto(co, fn, buf, LString("result"))
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	if state != ResumeOK || len(values) != 1 || values[0] != LString("result") {
		t.Fatalf("second resume = (%v, %v), want (ResumeOK, [result])", state, values)
	}
	if calls != 1 {
		t.Fatalf("yielding Go function invoked %d times, want 1", calls)
	}
}

func TestRootGoFunctionYieldIsResumable(t *testing.T) {
	L := NewState()
	defer L.Close()

	calls := 0
	fn := L.NewFunction(func(L *LState) int {
		calls++
		return L.Yield(LString("token"))
	})
	co := L.NewThreadWithContext(context.Background())

	state, values, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("first resume: %v", err)
	}
	if state != ResumeYield || len(values) != 1 || values[0] != LString("token") {
		t.Fatalf("first resume = (%v, %v), want (ResumeYield, [token])", state, values)
	}
	if co.stack.IsEmpty() || co.currentFrame == nil {
		t.Fatal("reported yield did not retain the root Go frame")
	}

	state, values, err = L.Resume(co, fn, LString("result"))
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	if state != ResumeOK || len(values) != 1 || values[0] != LString("result") {
		t.Fatalf("second resume = (%v, %v), want (ResumeOK, [result])", state, values)
	}
	if calls != 1 {
		t.Fatalf("root Go function invoked %d times, want 1", calls)
	}
	assertNoContinuationMetadata(t, co)
}

func TestRootGoFunctionYieldResumeInto(t *testing.T) {
	L := NewState()
	defer L.Close()

	calls := 0
	fn := L.NewFunction(func(L *LState) int {
		calls++
		return L.Yield(LString("token"))
	})
	co := L.NewThreadWithContext(context.Background())
	buf := make([]LValue, 0, 2)

	state, values, err := L.ResumeInto(co, fn, buf)
	if err != nil {
		t.Fatalf("first resume: %v", err)
	}
	if state != ResumeYield || len(values) != 1 || values[0] != LString("token") {
		t.Fatalf("first resume = (%v, %v), want (ResumeYield, [token])", state, values)
	}
	state, values, err = L.ResumeInto(co, fn, buf, LString("result"))
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	if state != ResumeOK || len(values) != 1 || values[0] != LString("result") {
		t.Fatalf("second resume = (%v, %v), want (ResumeOK, [result])", state, values)
	}
	if calls != 1 {
		t.Fatalf("root Go function invoked %d times, want 1", calls)
	}
	assertNoContinuationMetadata(t, co)
}

func TestTailCallYieldReturnsAllResumeValues(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("yield_api", L.NewFunction(func(L *LState) int {
		return L.Yield(LString("token"))
	}))
	if err := L.DoString(`tail = function() return yield_api("arg") end`); err != nil {
		t.Fatal(err)
	}

	fn := L.GetGlobal("tail").(*LFunction)
	co := L.NewThreadWithContext(context.Background())
	state, _, err := L.Resume(co, fn)
	if err != nil || state != ResumeYield {
		t.Fatalf("first resume = (%v, %v), want ResumeYield", state, err)
	}
	state, values, err := L.Resume(co, fn, LString("one"), LString("two"), LString("three"))
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	if state != ResumeOK || len(values) != 3 ||
		values[0] != LString("one") ||
		values[1] != LString("two") ||
		values[2] != LString("three") {
		t.Fatalf("second resume = (%v, %v), want (ResumeOK, [one two three])", state, values)
	}
}

func TestTailCallYieldThroughCoroutineResume(t *testing.T) {
	L := NewState()
	defer L.Close()

	calls := 0
	L.SetGlobal("yield_api", L.NewFunction(func(L *LState) int {
		calls++
		return L.Yield(LString("token"))
	}))
	if err := L.DoString(`
		function outer()
			local inner = coroutine.create(function()
				return yield_api("arg")
			end)
			local ok, value = coroutine.resume(inner)
			assert(ok, value)
			return value
		end
	`); err != nil {
		t.Fatal(err)
	}

	fn := L.GetGlobal("outer").(*LFunction)
	co := L.NewThreadWithContext(context.Background())
	state, values, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("first resume: %v", err)
	}
	if state != ResumeYield || len(values) != 1 || values[0] != LString("token") {
		t.Fatalf("first resume = (%v, %v), want (ResumeYield, [token])", state, values)
	}
	state, values, err = L.Resume(co, fn, LString("result"))
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	if state != ResumeOK || len(values) != 1 || values[0] != LString("result") {
		t.Fatalf("second resume = (%v, %v), want (ResumeOK, [result])", state, values)
	}
	if calls != 1 {
		t.Fatalf("yielding Go function invoked %d times, want 1", calls)
	}
}

func TestResumeRejectsYieldWithoutResumableFrame(t *testing.T) {
	for _, useResumeInto := range []bool{false, true} {
		name := "Resume"
		if useResumeInto {
			name = "ResumeInto"
		}
		t.Run(name, func(t *testing.T) {
			L := NewState()
			defer L.Close()

			fn := L.NewFunction(func(thread *LState) int {
				parent := thread.Parent
				thread.G.CurrentThread = parent
				thread.Parent = nil
				parent.Push(LTrue)
				parent.Push(LString("token"))

				thread.stack.Pop()
				thread.currentFrame = nil
				thread.yieldState = yieldSystem
				return -1
			})
			co := L.NewThreadWithContext(context.Background())

			var (
				state ResumeState
				err   error
			)
			if useResumeInto {
				state, _, err = L.ResumeInto(co, fn, make([]LValue, 0, 1))
			} else {
				state, _, err = L.Resume(co, fn)
			}
			if state != ResumeError || err == nil ||
				!strings.Contains(err.Error(), "yielded thread has no resumable frame") {
				t.Fatalf("resume = (%v, %v), want explicit unresumable-yield error", state, err)
			}
			if !co.Dead {
				t.Fatal("corrupt yielded thread remained restartable")
			}
		})
	}
}

func assertNoContinuationMetadata(t *testing.T, thread *LState) {
	t.Helper()
	for idx, ext := range thread.frameExt {
		if ext != nil && (ext.ErrFunc != nil || ext.Continuation != nil || ext.ContinuationCtx != nil) {
			t.Fatalf("stale frame metadata at index %d: %+v", idx, ext)
		}
	}
}
