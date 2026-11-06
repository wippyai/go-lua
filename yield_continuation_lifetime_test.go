package lua

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestYieldContinuationLifetime(t *testing.T) {
	cases := []struct {
		name, setup, operation string
		opcode                 int
		continuation           uint8
	}{
		{"getglobal", `set_environment(operation, setmetatable({}, {__index = meta}))`, `local v = missing`, OP_GETGLOBAL, yieldContGetField},
		{"gettable", ``, `local key = 1; local v = obj[key]`, OP_GETTABLE, yieldContGetField},
		{"getfield", ``, `local v = obj.missing`, OP_GETTABLEKS, yieldContGetField},
		{"getfield-go", `mt.__index = go_index`, `local v = obj.missing`, OP_GETTABLEKS, yieldContGetField},
		{"setglobal", `set_environment(operation, setmetatable({}, {__newindex = meta}))`, `missing = 1`, OP_SETGLOBAL, yieldContSetField},
		{"settable", ``, `local key = 1; obj[key] = 1`, OP_SETTABLE, yieldContSetField},
		{"setfield", ``, `obj.missing = 1`, OP_SETTABLEKS, yieldContSetField},
		{"self", ``, `obj:missing()`, OP_SELF, yieldContSelf},
		{"add", ``, `local v = obj + other`, OP_ADD, yieldContArith},
		{"sub", ``, `local v = obj - other`, OP_SUB, yieldContArith},
		{"mul", ``, `local v = obj * other`, OP_MUL, yieldContArith},
		{"div", ``, `local v = obj / other`, OP_DIV, yieldContArith},
		{"mod", ``, `local v = obj % other`, OP_MOD, yieldContArith},
		{"pow", ``, `local v = obj ^ other`, OP_POW, yieldContArith},
		{"unm", ``, `local v = -obj`, OP_UNM, yieldContUnm},
		{"len", ``, `local v = #obj`, OP_LEN, yieldContLen},
		{"concat", ``, `local v = obj .. other`, OP_CONCAT, yieldContConcat},
		{"eq", ``, `local v = obj == other`, OP_EQ, yieldContCompare},
		{"lt", ``, `local v = obj < other`, OP_LT, yieldContCompare},
		{"le", ``, `local v = obj <= other`, OP_LE, yieldContCompare},
		{"le-via-lt", `mt.__le = nil`, `local v = obj <= other`, OP_LE, yieldContCompare},
		{"iterator", ``, `for v in meta do end`, OP_TFORLOOP, yieldContTForLoop},
	}
	for _, tc := range cases {
		for _, growing := range []bool{false, true} {
			for _, exit := range []string{"pcall", "xpcall", "error", "close", "close-unpooled", "cancel"} {
				t.Run(fmt.Sprintf("%s/growing=%t/%s", tc.name, growing, exit), func(t *testing.T) {
					L := NewState(Options{MinimizeStackMemory: growing})
					defer L.Close()
					L.SetGlobal("go_yield", L.NewFunction(func(L *LState) int {
						return L.Yield(LString("waiting"))
					}))
					L.SetGlobal("go_index", L.NewFunction(func(L *LState) int {
						L.Push(L.GetGlobal("go_yield"))
						L.CallK(0, 2, func(L *LState, _ any, _ ResumeState) int {
							L.RaiseError("sentinel")
							return 0
						}, nil)
						return -1
					}))
					L.SetGlobal("set_environment", L.NewFunction(func(L *LState) int {
						L.CheckFunction(1).Env = L.CheckTable(2)
						return 0
					}))
					if err := L.DoString(`
						local function meta()
							local value, err = go_yield()
							if err then error(err) end
							return value
						end
						local mt = {}
						for _, name in ipairs({"__index", "__newindex", "__add", "__sub", "__mul",
							"__div", "__mod", "__pow", "__unm", "__len", "__concat", "__eq", "__lt", "__le"}) do
							mt[name] = meta
						end
						local obj, other = setmetatable({}, mt), setmetatable({}, mt)
						local function operation() ` + tc.operation + ` end
						` + tc.setup + `
						local function probe()
							local value = {answer = 42}
							assert(type(value) == "table")
							local key = "answer"
							assert(value[key] == 42)
							assert(value.answer == 42)
							return value
						end
						local function invoke()
							local value = probe()
							return value
						end
						function after()
							local value = invoke()
							return type(value), value.answer
						end
						function protected(kind)
							local ok, err
							if kind == "xpcall" then
								ok, err = xpcall(operation, function(err) return "handled:" .. err end)
							else
								ok, err = pcall(operation)
							end
							assert(ok == false and string.find(err, "sentinel", 1, true))
							local value = invoke()
							return type(value), value.answer
						end
						function unprotected() operation() end
					`); err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					co := L.NewThreadWithContext(ctx)
					defer co.Close()
					name := "unprotected"
					if exit == "pcall" || exit == "xpcall" {
						name = "protected"
					}
					fn := L.GetGlobal(name).(*LFunction)
					state, values, err := L.Resume(co, fn, LString(exit))
					if state != ResumeYield || err != nil || len(values) != 1 || values[0] != LString("waiting") {
						t.Fatalf("first resume = %v, %v, %v", state, values, err)
					}
					if co.yieldCont != tc.continuation {
						t.Fatalf("continuation = %d, want %d", co.yieldCont, tc.continuation)
					}
					owner := co.stack.At(int(co.yieldContIdx))
					if op := int(owner.Fn.Proto.Code[owner.Pc] >> 26); op != tc.opcode {
						t.Fatalf("suspended opcode = %d, want %d", op, tc.opcode)
					}
					switch exit {
					case "pcall", "xpcall", "error":
						state, values, err = L.Resume(co, fn, LNil, LString("sentinel"))
						if exit == "error" {
							if state != ResumeError || err == nil || !strings.Contains(err.Error(), "sentinel") {
								t.Errorf("error resume = %v, %v, %v", state, values, err)
							}
						} else if state != ResumeOK || err != nil || len(values) != 2 || values[0] != LString("table") || values[1] != LInteger(42) {
							t.Errorf("protected resume = %v, %v, %v", state, values, err)
						}
						assertNoYieldContinuation(t, co)
						if len(co.frameExt) != 0 {
							t.Errorf("finished frames retain extensions: %v", co.frameExt)
						}
					case "cancel":
						cancel()
						state, _, err = L.Resume(co, fn)
						if state != ResumeError || err == nil {
							t.Errorf("canceled resume = %v, %v", state, err)
						}
						assertNoYieldContinuation(t, co)
					case "close-unpooled":
						co.reg.forceResize(co.Options.RegistrySize + co.Options.RegistryGrowStep + 1)
					}
					co.Close()
					assertNoYieldContinuation(t, co)
					if exit == "close-unpooled" {
						return
					}
					// Supply the closed state even when the race detector drops pool entries.
					for statePool.Get() != nil {
					}
					newState := statePool.New
					statePool.New = func() any { return co }
					reused := L.NewThreadWithContext(context.Background())
					statePool.New = newState
					defer reused.Close()
					if reused != co {
						t.Fatal("closed state is not reused")
					}
					assertNoYieldContinuation(t, reused)
					state, values, err = L.Resume(reused, L.GetGlobal("after").(*LFunction))
					if state != ResumeOK || err != nil || len(values) != 2 || values[0] != LString("table") || values[1] != LInteger(42) {
						t.Fatalf("reused resume = %v, %v, %v", state, values, err)
					}
				})
			}
		}
	}
}

func TestYieldContinuationSurvivesInnerUnwind(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("go_yield", L.NewFunction(func(L *LState) int { return L.Yield() }))
	if err := L.DoString(`
		function run()
			local obj = setmetatable({}, {__index = function()
				go_yield()
				local ok = pcall(function() error("inner failure") end)
				assert(ok == false)
				return 42
			end})
			local value = obj.answer
			return value
		end
	`); err != nil {
		t.Fatal(err)
	}
	co := L.NewThreadWithContext(context.Background())
	defer co.Close()
	fn := L.GetGlobal("run").(*LFunction)
	state, _, err := L.Resume(co, fn)
	if state != ResumeYield || err != nil {
		t.Fatalf("first resume = %v, %v", state, err)
	}
	state, values, err := L.Resume(co, fn)
	if state != ResumeOK || err != nil || len(values) != 1 || values[0] != LInteger(42) {
		t.Fatalf("second resume = %v, %v, %v", state, values, err)
	}
	assertNoYieldContinuation(t, co)
}

func TestYieldContinuationDeepUnwind(t *testing.T) {
	L := NewState(Options{MinimizeStackMemory: true})
	defer L.Close()
	L.SetGlobal("go_yield", L.NewFunction(func(L *LState) int { return L.Yield() }))
	if err := L.DoString(`
		local obj = setmetatable({}, {__index = function()
			go_yield()
			error("sentinel")
		end})
		local function nested(depth)
			if depth == 0 then
				local value = obj.answer
				return value
			end
			local value = nested(depth - 1)
			return value
		end
		function run()
			local ok, err = pcall(nested, 16)
			assert(ok == false and string.find(err, "sentinel", 1, true))
			return 42
		end
	`); err != nil {
		t.Fatal(err)
	}
	co := L.NewThreadWithContext(context.Background())
	defer co.Close()
	fn := L.GetGlobal("run").(*LFunction)
	state, _, err := L.Resume(co, fn)
	if state != ResumeYield || err != nil || int(co.yieldContIdx) < FramesPerSegment*2 {
		t.Fatalf("first resume = %v, %v, owner=%d", state, err, co.yieldContIdx)
	}
	state, values, err := L.Resume(co, fn)
	if state != ResumeOK || err != nil || len(values) != 1 || values[0] != LInteger(42) {
		t.Fatalf("second resume = %v, %v, %v", state, values, err)
	}
	assertNoYieldContinuation(t, co)
}

func assertNoYieldContinuation(t *testing.T, L *LState) {
	t.Helper()
	if L.yieldCont != yieldContNone || L.yieldContRA != 0 || L.yieldContRB != 0 || L.yieldContIdx != 0 {
		t.Errorf("discarded continuation = (%d, %d, %d, %d), want zero", L.yieldCont, L.yieldContRA, L.yieldContRB, L.yieldContIdx)
	}
}

func TestYieldedProtectedCallFrameLifetime(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGlobal("go_yield", L.NewFunction(func(L *LState) int { return L.Yield() }))
	if err := L.DoString(`
		function run()
			local ok, value = xpcall(function()
				go_yield()
				return 42
			end, function(err) return "handled:" .. err end)
			assert(ok and value == 42)
			local caught, err = pcall(function()
				go_yield()
				error("sentinel", 0)
			end)
			assert(caught == false and err == "sentinel", err)
		end
	`); err != nil {
		t.Fatal(err)
	}
	co := L.NewThreadWithContext(context.Background())
	defer co.Close()
	fn := L.GetGlobal("run").(*LFunction)
	for i, want := range []ResumeState{ResumeYield, ResumeYield, ResumeOK} {
		state, _, err := L.Resume(co, fn)
		if state != want || err != nil {
			t.Fatalf("resume %d = %v, %v; want %v", i, state, err, want)
		}
	}
	if len(co.frameExt) != 0 {
		t.Fatalf("finished frames retain extensions: %v", co.frameExt)
	}
}

func TestWrappedYieldErrorFrameLifetime(t *testing.T) {
	L := NewState()
	defer L.Close()
	var child *LState
	L.SetGlobal("go_yield", L.NewFunction(func(L *LState) int {
		child = L
		return L.Yield()
	}))
	if err := L.DoString(`
		function run()
			local wrapped = coroutine.wrap(function()
				local obj = setmetatable({}, {__index = function()
					go_yield()
					error("sentinel")
				end})
				local value = obj.missing
				return value
			end)
			local ok, err = pcall(wrapped)
			assert(ok == false and string.find(err, "sentinel", 1, true))
		end
	`); err != nil {
		t.Fatal(err)
	}
	co := L.NewThreadWithContext(context.Background())
	defer co.Close()
	fn := L.GetGlobal("run").(*LFunction)
	for i, want := range []ResumeState{ResumeYield, ResumeOK} {
		state, _, err := L.Resume(co, fn)
		if state != want || err != nil {
			t.Fatalf("resume %d = %v, %v; want %v", i, state, err, want)
		}
	}
	if child == nil {
		t.Fatal("wrapped coroutine does not yield")
	}
	defer child.Close()
	assertNoYieldContinuation(t, child)
	if !child.Dead || child.yieldState != yieldNone || len(child.frameExt) != 0 {
		t.Fatalf("failed wrapped coroutine: dead=%t, yield=%d, extensions=%v", child.Dead, child.yieldState, child.frameExt)
	}
}
