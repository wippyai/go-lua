package lua

import (
	"context"
	"fmt"
	"runtime"
	"testing"
)

func TestPCallYieldGoFunctionRegisterPressureWippyShape(t *testing.T) {
	tests := []struct {
		name        string
		options     Options
		initialSize int
	}{
		{
			name:        "baseline",
			options:     Options{SkipOpenLibs: true},
			initialSize: RegistrySize,
		},
		{
			name: "minimum_configured",
			options: Options{
				RegistrySize:     128,
				RegistryMaxSize:  512,
				RegistryGrowStep: 8,
				SkipOpenLibs:     true,
			},
			initialSize: 128,
		},
		{
			name: "larger_configured",
			options: Options{
				RegistrySize:     384,
				RegistryMaxSize:  768,
				RegistryGrowStep: 64,
				SkipOpenLibs:     true,
			},
			initialSize: 384,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s_%d", test.name, test.initialSize), func(t *testing.T) {
			testPCallYieldGoFunctionRegisterPressureWippyShape(t, test.options, test.initialSize)
		})
	}
}

func testPCallYieldGoFunctionRegisterPressureWippyShape(t *testing.T, options Options, initialSize int) {
	for statePool.Get() != nil {
	}
	runtime.GC()
	for statePool.Get() != nil {
	}

	L := NewState(options)
	defer L.Close()
	for _, lib := range []luaLib{
		{BaseLibName, OpenBase},
		{TabLibName, OpenTable},
		{StringLibName, OpenString},
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
	grewRegistry := false
	L.SetGlobal("force_registry_growth", L.NewFunction(func(L *LState) int {
		oldCap := cap(L.reg.array)
		for cap(L.reg.array) == oldCap {
			L.Push(LNil)
		}
		grewRegistry = true
		return 0
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
				force_registry_growth()

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
	if !grewRegistry || cap(co.reg.array) <= initialSize {
		t.Fatalf("test did not force post-resume registry growth: cap=%d", cap(co.reg.array))
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
