package lua

import (
	"strings"
	"testing"
)

// TestXPCallDirectCallCatchesError verifies that xpcall catches errors when
// invoked in a direct (non-coroutine) context, i.e. under DoString/PCall.
//
// basePCall wraps CallK in its own defer/recover (baselib.go), so pcall works
// whether the outermost boundary is PCall or threadRun. baseXPCall historically
// relied on threadRun's recover via handleProtectedError, which is ONLY the
// boundary inside coroutines. Under DoString the panic hit PCall's recover
// first, errFunc never fired, and the error leaked past xpcall.
func TestXPCallDirectCallCatchesError(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local ok, err = xpcall(function() error("boom") end, function(e)
			return "handled: " .. tostring(e)
		end)
		assert(ok == false, "xpcall must return false on error")
		assert(type(err) == "string", "xpcall must return the handler's value")
		assert(string.find(err, "handled"), "xpcall must invoke the handler; got: " .. tostring(err))
	`)
	if err != nil {
		t.Fatalf("xpcall leaked the error past the call (bug): %v", err)
	}
}

// TestXPCallDirectCallPreservesSurroundingChunk verifies that a leaked xpcall
// error must not abort the rest of the chunk. Before the fix, the panic
// propagated through DoString and killed every statement after the xpcall.
func TestXPCallDirectCallPreservesSurroundingChunk(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local xpcall_ok, xpcall_err = xpcall(function() error("boom") end, function(e)
			return "handled"
		end)
		after_marker = "reached"
	`)
	if err != nil {
		t.Fatalf("xpcall error leaked and aborted the chunk: %v", err)
	}

	if got := L.GetGlobal("after_marker").String(); got != "reached" {
		t.Fatalf("statement after xpcall did not execute; after_marker=%q", got)
	}
}

// TestXPCallDirectErrorDoesNotSkipNextGoCall guards the protected-frame
// cleanup invariant. CallK stores xpcall's continuation on its call frame. If
// an error unwinds without clearing that metadata, the next Go-backed Lua call
// at the same stack depth resumes the stale xpcall continuation instead of
// invoking its own function.
func TestXPCallDirectErrorDoesNotSkipNextGoCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local ok, handled = xpcall(function()
			error("first")
		end, function(e)
			return "handled: " .. tostring(e)
		end)
		assert(ok == false)
		assert(string.find(handled, "handled"))

		local body_ran = false
		local next_ok, next_value = pcall(function()
			body_ran = true
			return "next-result"
		end)
		assert(next_ok == true, "next pcall must execute normally")
		assert(next_value == "next-result", "next pcall returned the wrong result")
		assert(body_ran == true, "next pcall body was skipped by a stale continuation")
	`)
	if err != nil {
		t.Fatalf("call after failed xpcall was corrupted: %v", err)
	}
}

// TestXPCallHandlerErrorDoesNotSkipNextGoCall covers the same cleanup path
// when the message handler itself raises. The handler's error must become the
// xpcall result, and no handler/continuation state may survive the return.
func TestXPCallHandlerErrorDoesNotSkipNextGoCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local ok, handled = xpcall(function()
			error("original")
		end, function()
			error("handler-failed")
		end)
		assert(ok == false)
		assert(string.find(tostring(handled), "handler%-failed"))

		local body_ran = false
		local next_ok = pcall(function()
			body_ran = true
		end)
		assert(next_ok == true)
		assert(body_ran == true, "handler failure left stale frame metadata")
	`)
	if err != nil {
		t.Fatalf("handler failure corrupted the next call: %v", err)
	}
}

func TestXPCallDirectErrorClearsFrameExtensions(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		xpcall(function() error("boom") end, function(e) return e end)
	`); err != nil {
		t.Fatal(err)
	}

	for idx, ext := range L.frameExt {
		if ext != nil && (ext.ErrFunc != nil || ext.Continuation != nil || ext.ContinuationCtx != nil) {
			t.Fatalf("stale protected-call metadata remains at frame index %d: %+v", idx, ext)
		}
	}
}

// TestPCallErrorClearsNestedFrameExtensions covers the general unwind path,
// not only xpcall's own frame. A Go function using CallK leaves continuation
// metadata on its frame when the called Lua function panics. The surrounding
// pcall must discard that metadata before the frame index is reused.
func TestPCallErrorClearsNestedFrameExtensions(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("call_with_continuation", L.NewFunction(func(L *LState) int {
		fn := L.CheckFunction(1)
		L.Push(fn)
		L.CallK(0, 0, func(*LState, any, ResumeState) int { return 0 }, nil)
		return 0
	}))

	markerCalled := false
	L.SetGlobal("mark_called", L.NewFunction(func(*LState) int {
		markerCalled = true
		return 0
	}))

	err := L.DoString(`
		local ok = pcall(function()
			call_with_continuation(function()
				error("nested failure")
			end)
		end)
		assert(ok == false)

		local next_ok = pcall(function()
			mark_called()
		end)
		assert(next_ok == true)
	`)
	if err != nil {
		t.Fatalf("nested protected-call unwind failed: %v", err)
	}
	if !markerCalled {
		t.Fatal("next Go call was skipped by a discarded frame's continuation")
	}
}

func TestAPIPCallErrorClearsNestedFrameExtensions(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("call_with_continuation", L.NewFunction(func(L *LState) int {
		fn := L.CheckFunction(1)
		L.Push(fn)
		L.CallK(0, 0, func(*LState, any, ResumeState) int { return 0 }, nil)
		return 0
	}))

	markerCalled := false
	L.SetGlobal("mark_called", L.NewFunction(func(*LState) int {
		markerCalled = true
		return 0
	}))

	if err := L.DoString(`
		function api_failure()
			call_with_continuation(function()
				error("api failure")
			end)
		end
		function api_next_call()
			mark_called()
		end
	`); err != nil {
		t.Fatal(err)
	}

	L.Push(L.GetGlobal("api_failure"))
	if err := L.PCall(0, 0, nil); err == nil {
		t.Fatal("expected API PCall to catch the nested error")
	}

	L.Push(L.GetGlobal("api_next_call"))
	if err := L.PCall(0, 0, nil); err != nil {
		t.Fatalf("next API PCall failed: %v", err)
	}
	if !markerCalled {
		t.Fatal("next API PCall was skipped by a discarded frame's continuation")
	}
}

// TestXPCallDirectCallReturnsTrueOnSuccess verifies the success path under
// direct call.
func TestXPCallDirectCallReturnsTrueOnSuccess(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local ok, val = xpcall(function() return "ok-value" end, function(e)
			return "should-not-run"
		end)
		assert(ok == true, "xpcall must return true on success")
		assert(val == "ok-value", "xpcall must return the function's results; got: " .. tostring(val))
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestXPCallNestedUnderPCall verifies xpcall also works when nested inside a
// pcall in a direct context (a common recovery pattern).
func TestXPCallNestedUnderPCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local outer_ok, outer_err = pcall(function()
			local ok, err = xpcall(function() error("inner") end, function(e)
				return "handled: " .. tostring(e)
			end)
			assert(ok == false, "xpcall should have caught the error")
			assert(string.find(tostring(err), "handled"), "handler should have run")
			return "completed"
		end)
		assert(outer_ok == true, "pcall must succeed because xpcall handled the error; got err: " .. tostring(outer_err))
		assert(outer_err == "completed", "unexpected outer return: " .. tostring(outer_err))
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestXPCallCoroutineStillWorks guards against the fix breaking the existing
// coroutine path (where xpcall was already handled via threadRun).
func TestXPCallCoroutineStillWorks(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function with_handler()
			local ok, err = xpcall(function() error("co-boom") end, function(e)
				return "co-handled: " .. tostring(e)
			end)
			return ok, err
		end
	`); err != nil {
		t.Fatal(err)
	}

	co, cancel := L.NewThread()
	defer cancel()
	fn := L.GetGlobal("with_handler").(*LFunction)

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("expected ResumeOK, got %v", state)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0] != LFalse {
		t.Errorf("expected xpcall false, got %v", results[0])
	}
	if !strings.Contains(results[1].String(), "co-handled") {
		t.Errorf("expected handler output, got %v", results[1])
	}
}
