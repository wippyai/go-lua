package lua

import (
	"context"
	"testing"
)

// TestPCallYieldGoFunctionTailReturn verifies the multret/tail-return form
// used by wrappers that return pcall(...) directly. A yielding Go function
// must resume as pcall's protected success, not leak its function object into
// the returned values.
func TestPCallYieldGoFunctionTailReturn(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("yielding_call", L.NewFunction(func(L *LState) int {
		return L.Yield(LString("request"))
	}))

	if err := L.DoString(`
		function run_tail_pcall()
			return pcall(yielding_call)
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.Background())
	fn := L.GetGlobal("run_tail_pcall").(*LFunction)

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("first resume failed: %v", err)
	}
	if state != ResumeYield || len(results) != 1 || results[0].String() != "request" {
		t.Fatalf("expected request yield, got state=%v results=%v", state, results)
	}

	state, results, err = L.Resume(co, fn, LString("response"))
	if err != nil {
		t.Fatalf("second resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("expected completion, got state=%v results=%v", state, results)
	}
	if len(results) != 2 || results[0] != LTrue || results[1].String() != "response" {
		t.Fatalf("expected [true response], got %v", results)
	}
}

// TestResumeRootGoFunction covers the other sole-Go-frame path: a Go function
// used directly as the resumed thread's entry point must transfer its results
// back to the resumer even though it was not reached through OP_TAILCALL.
func TestResumeRootGoFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	fn := L.NewFunction(func(L *LState) int {
		L.Push(LString("root-result"))
		return 1
	})
	co := L.NewThreadWithContext(context.Background())

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("expected completion, got state=%v results=%v", state, results)
	}
	if len(results) != 1 || results[0].String() != "root-result" {
		t.Fatalf("expected root-result, got %v", results)
	}
}
