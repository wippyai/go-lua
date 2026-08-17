package lua

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/stdlib"
)

// TestPCallYield tests that pcall is yield-transparent (Lua 5.3 behavior).
// When a function inside pcall yields, pcall should propagate the yield,
// and on resume, execution should continue inside pcall's protection.
func TestPCallYield(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test basic yield through pcall
	if err := L.DoString(`
		function test_pcall_yield()
			local ok, val = pcall(function()
				coroutine.yield("yielded")
				return "returned"
			end)
			return ok, val
		end
	`); err != nil {
		t.Fatal(err)
	}

	// Create coroutine
	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_pcall_yield").(*LFunction)

	// First resume - should yield "yielded"
	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("First resume failed: %v", err)
	}
	if state != ResumeYield {
		t.Fatalf("Expected ResumeYield, got %v", state)
	}
	if len(results) < 1 || results[0].String() != "yielded" {
		t.Fatalf("Expected yield value 'yielded', got %v", results)
	}

	// Second resume - should complete with (true, "returned")
	state, results, err = L.Resume(co, fn)
	if err != nil {
		t.Fatalf("Second resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if len(results) < 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	if results[0] != LTrue {
		t.Errorf("Expected pcall to return true, got %v", results[0])
	}
	if results[1].String() != "returned" {
		t.Errorf("Expected 'returned', got %v", results[1])
	}
}

// TestPCallYieldMultiple tests multiple yields through pcall.
func TestPCallYieldMultiple(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_multi_yield()
			local ok, val = pcall(function()
				local a = coroutine.yield(1)
				local b = coroutine.yield(2)
				return a + b
			end)
			return ok, val
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_multi_yield").(*LFunction)

	// First yield
	state, results, _ := L.Resume(co, fn)
	if state != ResumeYield || LVAsNumber(results[0]) != 1 {
		t.Fatalf("First yield: expected (yield, 1), got (%v, %v)", state, results)
	}

	// Second yield, send 10
	state, results, _ = L.Resume(co, fn, LNumber(10))
	if state != ResumeYield || LVAsNumber(results[0]) != 2 {
		t.Fatalf("Second yield: expected (yield, 2), got (%v, %v)", state, results)
	}

	// Complete, send 20 - should return (true, 30)
	state, results, _ = L.Resume(co, fn, LNumber(20))
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if results[0] != LTrue {
		t.Errorf("Expected pcall success, got %v", results[0])
	}
	if LVAsNumber(results[1]) != 30 {
		t.Errorf("Expected 30, got %v", results[1])
	}
}

// TestPCallYieldThenError tests that errors after yield are still caught.
func TestPCallYieldThenError(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_yield_error()
			local ok, val = pcall(function()
				coroutine.yield("before error")
				error("boom")
			end)
			return ok, val
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_yield_error").(*LFunction)

	// First resume - yields
	state, results, _ := L.Resume(co, fn)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}
	if results[0].String() != "before error" {
		t.Fatalf("Expected 'before error', got %v", results[0])
	}

	// Second resume - should return (false, error message)
	state, results, _ = L.Resume(co, fn)
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK (pcall caught error), got %v", state)
	}
	if results[0] != LFalse {
		t.Errorf("Expected pcall to return false (error caught), got %v", results[0])
	}
	// results[1] should contain error message with "boom"
}

// TestPCallYieldResumeValueCausesError tests error when using resumed value.
func TestPCallYieldResumeValueCausesError(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_resume_error()
			local ok, val = pcall(function()
				local x = coroutine.yield("waiting")
				-- x will be nil, calling it as function will error
				return x()
			end)
			return ok, val
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_resume_error").(*LFunction)

	// First resume - yields
	state, results, _ := L.Resume(co, fn)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}
	if results[0].String() != "waiting" {
		t.Fatalf("Expected 'waiting', got %v", results[0])
	}

	// Second resume with nil - using nil as function should error
	state, results, _ = L.Resume(co, fn) // no value sent, x will be nil
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK (pcall caught error), got %v", state)
	}
	if results[0] != LFalse {
		t.Errorf("Expected pcall to return false, got %v", results[0])
	}
}

// TestPCallYieldErrorAsSecondValue tests error returned as second value from yield (wippy pattern).
func TestPCallYieldErrorAsSecondValue(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_yield_err()
			local ok, result = pcall(function()
				-- Simulate: local id, err = coroutine.yield("request")
				local id, err = coroutine.yield("request")
				if err then
					error("got error: " .. tostring(err))
				end
				return "success: " .. tostring(id)
			end)
			return ok, result
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_yield_err").(*LFunction)

	// First resume - yields "request"
	state, results, _ := L.Resume(co, fn)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}
	if results[0].String() != "request" {
		t.Fatalf("Expected 'request', got %v", results[0])
	}

	// Resume with (nil, "connection failed") - error as second value
	state, results, _ = L.Resume(co, fn, LNil, LString("connection failed"))
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	// pcall should catch the error
	if results[0] != LFalse {
		t.Errorf("Expected false (error caught), got %v", results[0])
	}
	// Error message should contain "connection failed"
	if !strings.Contains(results[1].String(), "connection failed") {
		t.Errorf("Expected error containing 'connection failed', got %v", results[1])
	}

	// Test success path
	co2 := L.NewThreadWithContext(context.TODO())
	state, _, _ = L.Resume(co2, fn)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}

	// Resume with (123, nil) - success
	state, results, _ = L.Resume(co2, fn, LNumber(123), LNil)
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if results[0] != LTrue {
		t.Errorf("Expected true (success), got %v", results[0])
	}
	if results[1].String() != "success: 123" {
		t.Errorf("Expected 'success: 123', got %v", results[1])
	}
}

// TestPCallYieldErrorInResumedComputation tests error in computation with resumed value.
func TestPCallYieldErrorInResumedComputation(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_compute_error()
			local ok, val = pcall(function()
				local x = coroutine.yield("get value")
				-- force an error by calling error() based on resumed value
				if x == "fail" then
					error("intentional failure from resumed value")
				end
				return "success: " .. tostring(x)
			end)
			return ok, val
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_compute_error").(*LFunction)

	// First resume - yields
	state, _, _ := L.Resume(co, fn)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}

	// Resume with "fail" - should trigger error inside pcall
	state, results, _ := L.Resume(co, fn, LString("fail"))
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if results[0] != LFalse {
		t.Errorf("Expected false (error caught), got %v", results[0])
	}
	if results[1].String() == "" {
		t.Error("Expected error message")
	}

	// Now test success path
	co2 := L.NewThreadWithContext(context.TODO())
	state, _, _ = L.Resume(co2, fn)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}

	// Resume with "hello" - should succeed
	state, results, _ = L.Resume(co2, fn, LString("hello"))
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if results[0] != LTrue {
		t.Errorf("Expected true (success), got %v", results[0])
	}
	if results[1].String() != "success: hello" {
		t.Errorf("Expected 'success: hello', got %v", results[1])
	}
}

// TestGoFunctionYieldNoPcall tests Go function yield without pcall (baseline).
func TestGoFunctionYieldNoPcall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a Go function that yields
	L.SetGlobal("yielding_call", L.NewFunction(func(L *LState) int {
		return L.Yield(LString("yield_request"))
	}))

	if err := L.DoString(`
		function run_test()
			local result = yielding_call()
			return "got: " .. tostring(result)
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("run_test").(*LFunction)

	// First resume - yields
	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("First resume failed: %v", err)
	}
	if state != ResumeYield {
		t.Fatalf("Expected ResumeYield, got %v", state)
	}
	if results[0].String() != "yield_request" {
		t.Fatalf("Expected 'yield_request', got %v", results[0])
	}

	// Resume with value - should complete
	state, results, err = L.Resume(co, fn, LString("hello"))
	if err != nil {
		t.Fatalf("Second resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if results[0].String() != "got: hello" {
		t.Errorf("Expected 'got: hello', got %v", results[0])
	}
}

// TestPCallYieldGoFunction tests pcall with a Go function that yields (wippy runner pattern).
// Pattern: pcall(function() return go_func() end) where go_func yields
func TestPCallYieldGoFunction(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a Go function that yields
	L.SetGlobal("yielding_call", L.NewFunction(func(L *LState) int {
		// This simulates funcs.call which yields to scheduler
		return L.Yield(LString("yield_request"))
	}))

	if err := L.DoString(`
		function run_test()
			local ok, result, err = pcall(function()
				return yielding_call()
			end)
			return ok, result, err
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("run_test").(*LFunction)

	// First resume - should yield from yielding_call
	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("First resume failed: %v", err)
	}
	if state != ResumeYield {
		t.Fatalf("Expected ResumeYield, got %v", state)
	}
	if len(results) < 1 || results[0].String() != "yield_request" {
		t.Fatalf("Expected yield value 'yield_request', got %v", results)
	}
	t.Logf("After first yield: co.stack.Sp=%d, co.currentFrame=%p", co.stack.Sp(), co.currentFrame)
	if co.currentFrame != nil {
		t.Logf("  currentFrame: GoFunc=%v, Fn=%v", co.currentFrame.GoFunc != nil, co.currentFrame.Fn)
	}

	// Resume with success result - should complete with (true, "success_result", nil)
	state, results, err = L.Resume(co, fn, LString("success_result"))
	t.Logf("Second resume: state=%v, err=%v (type=%T), results=%v", state, err, err, results)
	t.Logf("co.Dead=%v, co.stack.IsEmpty=%v", co.Dead, co.stack.IsEmpty())
	if apiErr, ok := err.(*ApiError); ok {
		t.Logf("ApiError: Type=%v, Object=%v (type=%T), StackTrace=%v", apiErr.Type, apiErr.Object, apiErr.Object, apiErr.StackTrace)
	}
	if err != nil {
		t.Fatalf("Second resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results, got %d: %v", len(results), results)
	}
	if results[0] != LTrue {
		t.Errorf("Expected pcall to return true, got %v", results[0])
	}
	if results[1].String() != "success_result" {
		t.Errorf("Expected 'success_result', got %v", results[1])
	}
}

func TestPCallYieldGoFunctionRegisterPressureWippyShape(t *testing.T) {
	for statePool.Get() != nil {
	}
	runtime.GC()
	for statePool.Get() != nil {
	}

	L := NewState(Options{
		RegistrySize:     96,
		RegistryMaxSize:  512,
		RegistryGrowStep: 8,
		SkipOpenLibs:     true,
	})
	defer L.Close()
	for _, lib := range []luaLib{
		{stdlib.BaseName, OpenBase},
		{stdlib.TableName, OpenTable},
		{stdlib.StringName, OpenString},
	} {
		L.Push(L.NewFunction(LGFunction(lib.libFunc)))
		L.Push(LString(lib.libName))
		L.Call(1, 0)
	}

	L.SetGlobal("schedule_repo_create", L.NewFunction(func(L *LState) int {
		request := L.CheckTable(1)
		taskArgs, ok := request.RawGetString("task_args").(*LTable)
		if !ok {
			L.RaiseError("task_args disappeared before yield")
			return 0
		}
		if got := taskArgs.RawGetString("agent_ref"); got.String() != "test_agent" {
			L.RaiseError("agent_ref before yield = %s", got.String())
			return 0
		}
		return L.Yield(LString("db:begin"))
	}))

	if err := L.DoString(`
		local function pcall_wrap_install(body)
			local rollback_log = {}
			local tx = {
				rollback = function(kind, payload)
					rollback_log[#rollback_log + 1] = kind .. ":" .. payload.schedule_id
				end,
			}
			local ok, value = pcall(body, tx)
			if ok and value.state then
				value.rollback_count = #rollback_log
			end
			return ok, value
		end

		local function is_str(v)
			return type(v) == "string" and v ~= ""
		end

		function run_wippy_shape(input)
			return pcall_wrap_install(function(tx)
				input = type(input) == "table" and input or {}
				local title = is_str(input.title) and input.title or "repro"
				local schedule_type = "interval"
				local schedule_expression = "1h"
				local agent_id = nil
				local agent_ref = "test_agent"
				local now_str = "2026-06-23T12:00:00Z"
				local next_run = "2026-06-23T13:00:00Z"
				local actor_id = "repro_user"
				local actor_metadata = { security_groups = { "security:process" } }
				local user_id = is_str(input.user_id) and input.user_id or actor_id
				local actor_scope = actor_metadata.security_groups[#actor_metadata.security_groups]
				local user_ctx = type(input.context) == "table" and input.context or {}
				local task_args = {
					title = title,
					agent_id = agent_id,
					agent_ref = agent_ref,
					agent_title = nil,
					agent_icon = nil,
					kb_ids = nil,
					max_iterations = nil,
					actor_id = actor_id,
					actor_metadata = actor_metadata,
					actor_scope = actor_scope,
					schedule_type = schedule_type,
					schedule_expression = schedule_expression,
					next_run = next_run,
					now_str = now_str,
				}
				local schedule_id = schedule_repo_create({
					description = title,
					class = "automation",
					user_id = user_id,
					actor_id = actor_id,
					actor_scope = actor_scope,
					actor_metadata = actor_metadata,
					task_implementation_id = "x:y",
					task_args = task_args,
					task_context = user_ctx,
					schedule_type = schedule_type,
					schedule_expression = schedule_expression,
					next_run_at = next_run,
					timeout_seconds = 300,
					max_retries = 3,
					enabled = true,
				})

				tx.rollback("x:y", { schedule_id = schedule_id })
				local state = {}
				for k, v in pairs(user_ctx) do state[k] = v end
				state.schedule_id = schedule_id
				return {
					state = state,
					metadata = { title = title, actor_scope = actor_scope },
					task_args = task_args,
				}
			end)
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("run_wippy_shape").(*LFunction)
	input := L.NewTable()
	input.RawSetString("title", LString("first"))
	contextTable := L.NewTable()
	contextTable.RawSetString("trace_id", LString("trace-1"))
	input.RawSetString("context", contextTable)

	state, results, err := L.Resume(co, fn, input)
	if err != nil {
		t.Fatalf("first resume failed: %v", err)
	}
	if state != ResumeYield {
		t.Fatalf("expected yield, got %v with results %v", state, results)
	}
	if len(results) == 0 || results[0].String() != "db:begin" {
		t.Fatalf("expected db:begin yield, got %v", results)
	}

	state, results, err = L.Resume(co, fn, LString("schedule-123"))
	if err != nil {
		t.Fatalf("second resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("expected completion, got %v with results %v", state, results)
	}
	if len(results) < 2 || results[0] != LTrue {
		t.Fatalf("expected protected success, got %v", results)
	}
	result, ok := results[1].(*LTable)
	if !ok {
		t.Fatalf("expected result table, got %T %v", results[1], results[1])
	}
	stateTable := result.RawGetString("state").(*LTable)
	if got := stateTable.RawGetString("schedule_id"); got.String() != "schedule-123" {
		t.Fatalf("schedule_id = %v", got)
	}
	if got := stateTable.RawGetString("trace_id"); got.String() != "trace-1" {
		t.Fatalf("trace_id = %v", got)
	}
	taskArgs := result.RawGetString("task_args").(*LTable)
	if got := taskArgs.RawGetString("agent_ref"); got.String() != "test_agent" {
		t.Fatalf("agent_ref = %v", got)
	}
	metadata := result.RawGetString("metadata").(*LTable)
	if got := metadata.RawGetString("actor_scope"); got.String() != "security:process" {
		t.Fatalf("actor_scope = %v", got)
	}
	if got := result.RawGetString("rollback_count"); LVAsNumber(got) != 1 {
		t.Fatalf("rollback_count = %v", got)
	}
}

// TestPCallYieldGoFunctionMultiReturn tests pcall with Go function returning multiple values after yield.
func TestPCallYieldGoFunctionMultiReturn(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a Go function that yields and expects multiple return values
	L.SetGlobal("yielding_call", L.NewFunction(func(L *LState) int {
		return L.Yield(LString("yield_request"))
	}))

	if err := L.DoString(`
		function run_test()
			local ok, result, err = pcall(function()
				return yielding_call()
			end)
			return ok, result, err
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("run_test").(*LFunction)

	// First resume - yields
	state, _, _ := L.Resume(co, fn)
	if state != ResumeYield {
		t.Fatalf("Expected ResumeYield, got %v", state)
	}

	// Resume with (result, error) pattern
	state, results, err := L.Resume(co, fn, LString("the_result"), LString("the_error"))
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	// pcall should return: true, "the_result", "the_error"
	if results[0] != LTrue {
		t.Errorf("Expected true, got %v", results[0])
	}
	if results[1].String() != "the_result" {
		t.Errorf("Expected 'the_result', got %v", results[1])
	}
	if len(results) > 2 && results[2].String() != "the_error" {
		t.Errorf("Expected 'the_error', got %v", results[2])
	}
}

// TestPCallNestedYield tests nested pcalls with yields.
func TestPCallNestedYield(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_nested()
			local ok1, val1 = pcall(function()
				local ok2, val2 = pcall(function()
					coroutine.yield("inner")
					return "inner_done"
				end)
				return ok2, val2
			end)
			return ok1, val1
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_nested").(*LFunction)

	// First resume - yields from inner pcall
	state, results, _ := L.Resume(co, fn)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}
	if results[0].String() != "inner" {
		t.Fatalf("Expected 'inner', got %v", results[0])
	}

	// Second resume - completes both pcalls
	state, results, _ = L.Resume(co, fn)
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	// Should return (true, true, "inner_done") - outer pcall result
	if results[0] != LTrue {
		t.Errorf("Expected outer pcall true, got %v", results[0])
	}
}

// TestXPCallYield tests xpcall with yield.
func TestXPCallYield(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		local error_handler_called = false
		function handler(err)
			error_handler_called = true
			return "handled: " .. tostring(err)
		end

		function test_xpcall_yield()
			print("entering test_xpcall_yield")
			local ok, val = xpcall(function()
				print("in xpcall fn, before yield")
				coroutine.yield("yielded")
				print("in xpcall fn, after yield, about to error")
				error("test error")
			end, handler)
			print("after xpcall, ok=", ok, "val=", val)
			return ok, val, error_handler_called
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_xpcall_yield").(*LFunction)

	// First resume - yields
	state, results, _ := L.Resume(co, fn)
	t.Logf("First resume: state=%v results=%v", state, results)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}

	// Second resume - error should be caught by handler
	state, results, _ = L.Resume(co, fn)
	t.Logf("Second resume: state=%v results=%v", state, results)
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if results[0] != LFalse {
		t.Errorf("Expected xpcall to return false, got %v", results[0])
	}
	// results[2] should be true (handler was called)
}

// TestGoFunctionYieldThroughPCall tests Go functions that yield through pcall.
func TestGoFunctionYieldThroughPCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Register a Go function that yields
	L.SetGlobal("my_yield", L.NewFunction(func(L *LState) int {
		return L.Yield(L.Get(1))
	}))

	if err := L.DoString(`
		function test_go_yield()
			local ok, val = pcall(function()
				my_yield("from go")
				return "after yield"
			end)
			return ok, val
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test_go_yield").(*LFunction)

	// First resume - yields via Go function
	state, results, _ := L.Resume(co, fn)
	if state != ResumeYield {
		t.Fatalf("Expected yield, got %v", state)
	}
	if results[0].String() != "from go" {
		t.Fatalf("Expected 'from go', got %v", results[0])
	}

	// Second resume - completes
	state, results, _ = L.Resume(co, fn)
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if results[0] != LTrue {
		t.Errorf("Expected true, got %v", results[0])
	}
	if results[1].String() != "after yield" {
		t.Errorf("Expected 'after yield', got %v", results[1])
	}
}

// TestPCallMultipleErrorsInCoroutine tests that multiple errors in sequence are all caught by pcall.
func TestPCallMultipleErrorsInCoroutine(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("raise_error", L.NewFunction(func(L *LState) int {
		msg := L.CheckString(1)
		L.RaiseError(msg)
		return 0
	}))

	if err := L.DoString(`
		function run_tests()
			local ok1, err1 = pcall(function()
				raise_error("first error")
			end)

			local ok2, err2 = pcall(function()
				raise_error("second error")
			end)

			local ok3, result3 = pcall(function()
				return "success"
			end)

			return ok1, tostring(err1), ok2, tostring(err2), ok3, result3
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("run_tests").(*LFunction)

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}

	if len(results) < 6 {
		t.Fatalf("Expected 6 results, got %d: %v", len(results), results)
	}

	if results[0] != LFalse {
		t.Errorf("First pcall should return false, got %v", results[0])
	}
	if !strings.Contains(results[1].String(), "first error") {
		t.Errorf("First error should contain 'first error', got %v", results[1])
	}

	if results[2] != LFalse {
		t.Errorf("Second pcall should return false, got %v", results[2])
	}
	if !strings.Contains(results[3].String(), "second error") {
		t.Errorf("Second error should contain 'second error', got %v", results[3])
	}

	if results[4] != LTrue {
		t.Errorf("Third pcall should return true, got %v", results[4])
	}
	if results[5].String() != "success" {
		t.Errorf("Third result should be 'success', got %v", results[5])
	}
}

// TestPCallErrorInCoroutine tests that pcall correctly catches errors
// when running inside a coroutine. This verifies that pcall returns
// (false, error_message) rather than propagating the error.
func TestPCallErrorInCoroutine(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_func()
			local function will_error()
				error("test error message")
			end

			local function has_credentials()
				return pcall(will_error)
			end

			local result = has_credentials()
			-- pcall should return false on error, not the error string
			if type(result) ~= "boolean" then
				return "FAIL: pcall returned " .. type(result) .. " instead of boolean"
			end
			if result ~= false then
				return "FAIL: pcall returned true instead of false"
			end
			return "SUCCESS"
		end
	`); err != nil {
		t.Fatal(err)
	}

	// Run inside a coroutine (this is how wippy executes Lua code)
	co, cancel := L.NewThread()
	defer cancel()
	fn := L.GetGlobal("test_func").(*LFunction)

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if len(results) < 1 {
		t.Fatal("Expected at least 1 result")
	}
	result := results[0].String()
	if result != "SUCCESS" {
		t.Errorf("Test failed: %s", result)
	}
}

// TestPCallErrorReturnValuesInCoroutine tests that pcall returns both
// (false, error_message) correctly when running inside a coroutine.
func TestPCallErrorReturnValuesInCoroutine(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_func()
			local function will_error()
				error("my error message")
			end

			-- Capture both return values in a table
			local results = {pcall(will_error)}

			-- Check first return value is false
			if results[1] ~= false then
				return "FAIL: first value is not false"
			end

			-- Check second value exists
			if results[2] == nil then
				return "FAIL: second value is nil"
			end

			return "SUCCESS"
		end
	`); err != nil {
		t.Fatal(err)
	}

	co, cancel := L.NewThread()
	defer cancel()
	fn := L.GetGlobal("test_func").(*LFunction)

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if len(results) < 1 {
		t.Fatal("Expected at least 1 result")
	}
	result := results[0].String()
	if result != "SUCCESS" {
		t.Errorf("Test failed: %s", result)
	}
}

// TestPCallErrorNotIfCondition verifies the practical use case:
// if not pcall(...) then should enter the block when pcall fails.
func TestPCallErrorNotIfCondition(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function test_func()
			local function will_error()
				error("credentials missing")
			end

			local function has_credentials()
				return pcall(will_error)
			end

			-- This is the common pattern: if not has_credentials() then ...
			if not has_credentials() then
				return "ENTERED_IF_BLOCK"
			else
				return "SKIPPED_IF_BLOCK"
			end
		end
	`); err != nil {
		t.Fatal(err)
	}

	co, cancel := L.NewThread()
	defer cancel()
	fn := L.GetGlobal("test_func").(*LFunction)

	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v", state)
	}
	if len(results) < 1 {
		t.Fatal("Expected at least 1 result")
	}
	result := results[0].String()
	if result != "ENTERED_IF_BLOCK" {
		t.Errorf("Expected 'ENTERED_IF_BLOCK' but got '%s' - pcall error handling broken in coroutine", result)
	}
}

// Tests that failed pcall does not pollute state pool, ensuring subsequent
// pooled LStates receive correct return values from Go functions.
func TestPcallErrorDoesNotPolluteStatePool(t *testing.T) {
	returnNilNil := func(L *LState) int {
		L.Push(LNil)
		L.Push(LNil)
		return 2
	}

	t.Run("before_pcall", func(t *testing.T) {
		L := NewState()
		defer L.Close()
		L.SetGlobal("test_func", L.NewFunction(returnNilNil))

		err := L.DoString(`
			local r, e = test_func()
			if r ~= nil then error("expected nil, got " .. type(r)) end
			if e ~= nil then error("expected nil error, got " .. type(e)) end
		`)
		if err != nil {
			t.Fatalf("before pcall: %v", err)
		}
	})

	t.Run("pcall_with_error", func(t *testing.T) {
		L := NewState()
		defer L.Close()

		err := L.DoString(`
			local success = pcall(function()
				error("intentional error")
			end)
		`)
		if err != nil {
			t.Fatalf("pcall test: %v", err)
		}
	})

	t.Run("after_pcall", func(t *testing.T) {
		L := NewState()
		defer L.Close()
		L.SetGlobal("test_func", L.NewFunction(returnNilNil))

		err := L.DoString(`
			local r, e = test_func()
			if r ~= nil then error("expected nil, got " .. type(r)) end
			if e ~= nil then error("expected nil error, got " .. type(e)) end
		`)
		if err != nil {
			t.Fatalf("after pcall: %v", err)
		}
	})
}

// TestPcallSuccessDoesNotPollute verifies that successful pcall doesn't cause pollution
func TestPcallSuccessDoesNotPollute(t *testing.T) {
	returnNilNil := func(L *LState) int {
		L.Push(LNil)
		L.Push(LNil)
		return 2
	}

	// Run successful pcall
	t.Run("pcall_success", func(t *testing.T) {
		L := NewState()
		defer L.Close()

		err := L.DoString(`
			local success = pcall(function()
				return 42
			end)
		`)
		if err != nil {
			t.Fatalf("pcall test: %v", err)
		}
	})

	// This should still work
	t.Run("after_successful_pcall", func(t *testing.T) {
		L := NewState()
		defer L.Close()
		L.SetGlobal("test_func", L.NewFunction(returnNilNil))

		err := L.DoString(`
			local r, e = test_func()
			if r ~= nil then error("expected nil, got " .. type(r)) end
		`)
		if err != nil {
			t.Fatalf("after successful pcall: %v", err)
		}
	})
}

// TestPcallErrorWithGoFunctionCallAfter verifies that calling a Go function
// after pcall prevents the pollution
func TestPcallErrorWithGoFunctionCallAfter(t *testing.T) {
	returnNilNil := func(L *LState) int {
		L.Push(LNil)
		L.Push(LNil)
		return 2
	}

	// Run pcall with error, but call print() after (Go function)
	t.Run("pcall_with_gofunc_after", func(t *testing.T) {
		L := NewState()
		defer L.Close()

		err := L.DoString(`
			local success = pcall(function()
				error("intentional error")
			end)
			print(success)  -- This Go function call prevents pollution
		`)
		if err != nil {
			t.Fatalf("pcall test: %v", err)
		}
	})

	// This should work because we called print() after pcall
	t.Run("after_pcall_with_gofunc", func(t *testing.T) {
		L := NewState()
		defer L.Close()
		L.SetGlobal("test_func", L.NewFunction(returnNilNil))

		err := L.DoString(`
			local r, e = test_func()
			if r ~= nil then error("expected nil, got " .. type(r)) end
		`)
		if err != nil {
			t.Fatalf("after pcall with gofunc: %v", err)
		}
	})
}

// TestPooledStateYieldReset_ResetLState verifies that resetLState
// (called during Close/pool return) clears the yield state.
func TestPooledStateYieldReset_ResetLState(t *testing.T) {
	L := NewState()
	L.yieldState = yieldSystem
	resetLState(L)
	if L.yieldState != yieldNone {
		t.Fatal("resetLState must clear yieldState")
	}
}

// TestPooledStateYieldReset_NewLState verifies that newLState
// clears yieldState on a state retrieved from the pool.
func TestPooledStateYieldReset_NewLState(t *testing.T) {
	for statePool.Get() != nil {
	}

	dirty := NewState()
	dirty.yieldState = yieldSystem
	statePool.Put(dirty)

	reused := NewState()
	defer reused.Close()
	if reused.yieldState != yieldNone {
		t.Fatal("newLState must reset yieldState on pooled state")
	}
}

// TestPooledStateYieldReset_NewLStateWithGAndAlloc verifies that
// newLStateWithGAndAlloc clears yieldState on a state retrieved from the pool.
func TestPooledStateYieldReset_NewLStateWithGAndAlloc(t *testing.T) {
	for statePool.Get() != nil {
	}

	d1 := NewState()
	d1.yieldState = yieldSystem
	d2 := NewState()
	d2.yieldState = yieldUser
	statePool.Put(d1)
	statePool.Put(d2)

	parent := NewState()
	defer parent.Close()
	thread := parent.NewThreadWithContext(context.TODO())
	if thread.yieldState != yieldNone {
		t.Fatal("newLStateWithGAndAlloc must reset yieldState on pooled state")
	}
}

// TestPooledStateYieldedFlagReset_EndToEnd simulates the real-world scenario:
// a coroutine yields, its state is pooled, then reused for a new execution.
func TestPooledStateYieldedFlagReset_EndToEnd(t *testing.T) {
	for statePool.Get() != nil {
	}

	// Phase 1: Yield inside a coroutine, then close the parent to pool everything
	func() {
		L := NewState()
		if err := L.DoString(`
			function yielder()
				coroutine.yield("y")
				return "done"
			end
		`); err != nil {
			t.Fatal(err)
		}
		co := L.NewThreadWithContext(context.TODO())
		fn := L.GetGlobal("yielder").(*LFunction)
		state, _, err := L.Resume(co, fn)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		if state != ResumeYield {
			t.Fatalf("expected ResumeYield, got %v", state)
		}
		if co.yieldState == yieldNone {
			t.Fatal("co.yieldState must be non-zero after yield")
		}
		L.Close()
	}()

	// Phase 2: Reuse pooled states - everything must work correctly
	L := NewState()
	defer L.Close()

	if err := L.DoString(`
		function compute()
			local ok, val = pcall(function()
				return "result"
			end)
			if not ok then error("pcall failed") end
			return val
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("compute").(*LFunction)
	state, results, err := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("expected ResumeOK, got %v", state)
	}
	if len(results) < 1 || results[0].String() != "result" {
		t.Fatalf("expected 'result', got %v", results)
	}
}
