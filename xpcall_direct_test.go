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
