package lua

import (
	"context"
	"testing"
)

const upvalueCloseCases = `
local function clobber(p, q, r, s, t, u) return p, q, r, s, t, u end

local function writer()
	local value
	return function(x) value = x end, function() return value end
end

function check_caught_error_keeps_caller_upvalues()
	local value
	local function set(x) value = x end
	pcall(error, "first")
	set(42)
	assert(value == 42, "pcall: lost closure write, got " .. tostring(value))
end

function check_nested_caught_errors()
	local value
	local function set(x) value = x end
	pcall(function()
		pcall(error, "first")
		set(42)
		error("second")
	end)
	assert(value == 42, "nested pcall: lost closure write, got " .. tostring(value))
end

function check_xpcall()
	local value
	local function set(x) value = x end
	xpcall(function() error("first") end, function(e) return e end)
	set(42)
	assert(value == 42, "xpcall: lost closure write, got " .. tostring(value))
end

function check_error_object()
	local value
	local function set(x) value = x end
	pcall(error, {})
	set(42)
	assert(value == 42, "error object: lost closure write, got " .. tostring(value))
end

function check_runtime_error()
	local value
	local function set(x) value = x end
	pcall(function() local missing = nil; return missing.field end)
	set(42)
	assert(value == 42, "runtime error: lost closure write, got " .. tostring(value))
end

function check_shared_upvalue()
	local count = 0
	local function increment() count = count + 1 end
	local function read() return count end
	pcall(error, "first")
	increment()
	count = count + 1
	assert(read() == 2 and count == 2, "shared upvalue split after caught error")
end

function check_go_pcall()
	local value
	local function set(x) value = x end
	assert(go_pcall(function() error("first") end) == false)
	set(42)
	assert(value == 42, "Go PCall: lost closure write, got " .. tostring(value))
end

function check_escaped_closure()
	local saved
	pcall(function()
		local kept = "kept"
		saved = function() return kept end
		error("first")
	end)
	clobber("a", "b", "c", "d", "e", "f")
	assert(saved() == "kept", "pcall: escaped closure lost its value")
end

function check_escaped_closure_go_pcall_handler()
	local saved
	assert(go_pcall_handler(function()
		local kept = "kept"
		saved = function() return kept end
		error("first")
	end) == false)
	clobber("a", "b", "c", "d", "e", "f")
	assert(saved() == "kept", "Go PCall with handler: escaped closure lost its value")
end

function check_escaped_closure_xpcall()
	local saved
	xpcall(function()
		local kept = "kept"
		saved = function() return kept end
		error("first")
	end, function(e) return e end)
	clobber("a", "b", "c", "d", "e", "f")
	assert(saved() == "kept", "xpcall: escaped closure lost its value")
end

function check_escaped_closure_go_pcall_failing_handler()
	local saved
	assert(go_pcall_failing_handler(function()
		local kept = "kept"
		saved = function() return kept end
		error("first")
	end) == false)
	clobber("a", "b", "c", "d", "e", "f")
	assert(saved() == "kept", "Go PCall with failing handler: escaped closure lost its value")
end

function check_escaped_closure_dead_coroutine()
	local saved
	local co = coroutine.create(function()
		local kept = "kept"
		saved = function() return kept end
		error("dead")
	end)
	assert(coroutine.resume(co) == false)
	clobber("a", "b", "c", "d", "e", "f")
	assert(saved() == "kept", "dead coroutine: escaped closure lost its value")
end

function check_independent_writers()
	local set, get = writer()
	pcall(error, "first")
	set(42)
	assert(get() == 42, "independent writer lost its value")
end

function check_escaped_closure_xpcall_failing_handler()
	local saved
	local ok = xpcall(function()
		local kept = "kept"
		saved = function() return kept end
		error("first")
	end, function() error("handler failed") end)
	assert(ok == false)
	clobber("a", "b", "c", "d", "e", "f")
	assert(saved() == "kept", "xpcall with failing handler: escaped closure lost its value")
end

function check_xpcall_handler_writes_caller_upvalue()
	local value
	local function set(x) value = x end
	xpcall(function() error("first") end, function(e) set("handled"); return e end)
	assert(value == "handled", "xpcall handler: lost closure write, got " .. tostring(value))
	set(42)
	assert(value == 42, "xpcall handler: lost later closure write, got " .. tostring(value))
end

function check_escaped_closure_wrapped_dead_coroutine()
	local value
	local function set(x) value = x end
	local saved
	local ok = pcall(coroutine.wrap(function()
		local kept = "kept"
		saved = function() return kept end
		error("dead")
	end))
	assert(ok == false)
	set(42)
	clobber("a", "b", "c", "d", "e", "f")
	assert(value == 42, "wrapped coroutine: lost closure write, got " .. tostring(value))
	assert(saved() == "kept", "wrapped coroutine: escaped closure lost its value")
end

function check_go_raise_error()
	local value
	local function set(x) value = x end
	assert(pcall(go_raise_error) == false)
	set(42)
	assert(value == 42, "Go RaiseError: lost closure write, got " .. tostring(value))
end

function check_inner_catch_keeps_enclosing_frame_open()
	local outer
	local ok = pcall(function()
		local value
		local function set(x) value = x end
		pcall(error, "first")
		set(42)
		outer = value
	end)
	assert(ok == true and outer == 42, "enclosing protected frame lost closure write, got " .. tostring(outer))
end

function check_yield_then_error()
	local value
	local function set(x) value = x end
	local saved
	local ok = pcall(function()
		local kept = "kept"
		saved = function() return kept end
		coroutine.yield("pause")
		error("after yield")
	end)
	assert(ok == false)
	set(42)
	clobber("a", "b", "c", "d", "e", "f")
	assert(value == 42, "pcall after yield: lost closure write, got " .. tostring(value))
	assert(saved() == "kept", "pcall after yield: escaped closure lost its value")
end
`

var upvalueCloseCaseNames = []string{
	"check_caught_error_keeps_caller_upvalues",
	"check_nested_caught_errors",
	"check_xpcall",
	"check_error_object",
	"check_runtime_error",
	"check_shared_upvalue",
	"check_go_pcall",
	"check_escaped_closure",
	"check_escaped_closure_go_pcall_handler",
	"check_escaped_closure_xpcall",
	"check_escaped_closure_go_pcall_failing_handler",
	"check_escaped_closure_dead_coroutine",
	"check_independent_writers",
	"check_escaped_closure_xpcall_failing_handler",
	"check_xpcall_handler_writes_caller_upvalue",
	"check_escaped_closure_wrapped_dead_coroutine",
	"check_go_raise_error",
	"check_inner_catch_keeps_enclosing_frame_open",
}

func newUpvalueCloseState(t *testing.T) *LState {
	t.Helper()
	L := NewState()
	L.SetGlobal("go_pcall", L.NewFunction(func(L *LState) int {
		L.Push(L.CheckFunction(1))
		L.Push(LBool(L.PCall(0, 0, nil) == nil))
		return 1
	}))
	L.SetGlobal("go_pcall_handler", L.NewFunction(func(L *LState) int {
		L.Push(L.CheckFunction(1))
		handler := L.NewFunction(func(L *LState) int { return 1 })
		L.Push(LBool(L.PCall(0, 0, handler) == nil))
		return 1
	}))
	L.SetGlobal("go_pcall_failing_handler", L.NewFunction(func(L *LState) int {
		L.Push(L.CheckFunction(1))
		handler := L.NewFunction(func(L *LState) int {
			L.RaiseError("handler failed")
			return 0
		})
		L.Push(LBool(L.PCall(0, 0, handler) == nil))
		return 1
	}))
	L.SetGlobal("go_raise_error", L.NewFunction(func(L *LState) int {
		L.RaiseError("go failed")
		return 0
	}))
	if err := L.DoString(upvalueCloseCases); err != nil {
		L.Close()
		t.Fatal(err)
	}
	return L
}

func TestCaughtErrorKeepsSurvivingUpvaluesOpen(t *testing.T) {
	L := newUpvalueCloseState(t)
	defer L.Close()

	for _, name := range upvalueCloseCaseNames {
		if err := L.DoString(name + "()"); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCaughtErrorKeepsSurvivingUpvaluesOpenInCoroutine(t *testing.T) {
	L := newUpvalueCloseState(t)
	defer L.Close()

	for _, name := range append(upvalueCloseCaseNames, "check_yield_then_error") {
		co := L.NewThreadWithContext(context.TODO())
		fn := L.GetGlobal(name).(*LFunction)
		state, _, err := L.Resume(co, fn)
		for err == nil && state == ResumeYield {
			state, _, err = L.Resume(co, fn)
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
		} else if state != ResumeOK {
			t.Errorf("%s: expected ResumeOK, got %v", name, state)
		}
	}
}

func TestUncaughtErrorClosesEscapedUpvalues(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		local kept = "kept"
		saved = function() return kept end
		error("uncaught")
	`); err == nil {
		t.Fatal("expected uncaught error")
	}
	if err := L.DoString(`
		local a, b, c, d, e, f = "a", "b", "c", "d", "e", "f"
		assert(saved() == "kept", "uncaught error: escaped closure read " .. tostring(saved()))
	`); err != nil {
		t.Fatal(err)
	}
}

func TestDeadCoroutineClosesEscapedUpvalues(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function escape_and_fail()
			local kept = "kept"
			saved = function() return kept end
			error("dead")
		end
		function overwrite()
			local a, b, c, d, e, f = "a", "b", "c", "d", "e", "f"
			return a, b, c, d, e, f
		end
	`); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 8; i++ {
		co := L.NewThreadWithContext(context.TODO())
		if _, _, err := L.Resume(co, L.GetGlobal("escape_and_fail").(*LFunction)); err == nil {
			t.Fatal("expected coroutine error")
		}
		saved := L.GetGlobal("saved")
		co.Close()

		reused := L.NewThreadWithContext(context.TODO())
		if _, _, err := L.Resume(reused, L.GetGlobal("overwrite").(*LFunction)); err != nil {
			t.Fatal(err)
		}
		L.Push(saved)
		if err := L.PCall(0, 1, nil); err != nil {
			t.Fatal(err)
		}
		if got := L.Get(-1); got != LString("kept") {
			t.Fatalf("iteration %d: escaped closure read %v after thread reuse", i, got)
		}
		L.Pop(1)
		reused.Close()
	}
}

const pcallUpvalueBenchmarkScript = `
local counter = 0
local function increment() counter = counter + 1 end
local function fail() error("expected") end
local function succeed() return true end
local function deep(depth)
	local captured = depth
	local function touch() captured = captured + 1 end
	if depth == 0 then error("expected") end
	touch()
	return deep(depth - 1) + captured
end
function bench_caught_error()
	for _ = 1, 100 do
		pcall(fail)
		increment()
	end
	return counter
end
function bench_caught_deep_error()
	for _ = 1, 100 do
		pcall(deep, 8)
		increment()
	end
	return counter
end
function bench_success()
	for _ = 1, 100 do
		pcall(succeed)
		increment()
	end
	return counter
end
`

func benchmarkPCallUpvalues(b *testing.B, name string, coroutine bool) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(pcallUpvalueBenchmarkScript); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal(name).(*LFunction)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if coroutine {
			co, cancel := L.NewThread()
			if _, _, err := L.Resume(co, fn); err != nil {
				b.Fatal(err)
			}
			cancel()
			continue
		}
		L.Push(fn)
		if err := L.PCall(0, 1, nil); err != nil {
			b.Fatal(err)
		}
		L.Pop(1)
	}
}

func BenchmarkPCallCaughtErrorUpvalues(b *testing.B) {
	benchmarkPCallUpvalues(b, "bench_caught_error", false)
}

func BenchmarkPCallCaughtDeepErrorUpvalues(b *testing.B) {
	benchmarkPCallUpvalues(b, "bench_caught_deep_error", false)
}

func BenchmarkPCallSuccessUpvalues(b *testing.B) {
	benchmarkPCallUpvalues(b, "bench_success", false)
}

func BenchmarkPCallCaughtErrorUpvaluesCoroutine(b *testing.B) {
	benchmarkPCallUpvalues(b, "bench_caught_error", true)
}

func BenchmarkPCallCaughtDeepErrorUpvaluesCoroutine(b *testing.B) {
	benchmarkPCallUpvalues(b, "bench_caught_deep_error", true)
}

func BenchmarkPCallSuccessUpvaluesCoroutine(b *testing.B) {
	benchmarkPCallUpvalues(b, "bench_success", true)
}
