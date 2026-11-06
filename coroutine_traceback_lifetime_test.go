package lua

import (
	"fmt"
	"strings"
	"testing"
)

func TestFailedCoroutineTraceback(t *testing.T) {
	for _, growing := range []bool{false, true} {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("growing=%t/wrapped=%t", growing, wrapped), func(t *testing.T) {
				// Create both states with the selected stack implementation.
				for statePool.Get() != nil {
				}
				L := NewState(Options{MinimizeStackMemory: growing})
				defer L.Close()
				var child *LState
				L.SetGlobal("capture_thread", L.NewFunction(func(co *LState) int {
					child = co
					L.SetGlobal("failed_thread", co)
					return 0
				}))
				L.SetGlobal("wrapped", LBool(wrapped))
				fn, err := L.Load(strings.NewReader(`local function inner() error("boom") end
local function outer() local value = {answer = 42}; saved = function() return value end; inner() end
local function body() capture_thread(); outer() end
local ok, err
if wrapped then
    ok, err = pcall(coroutine.wrap(body))
else
    ok, err = coroutine.resume(coroutine.create(body))
end
assert(ok == false and string.find(err, "boom", 1, true))
trace = debug.traceback(failed_thread)
`), "@traceback.lua")
				if err != nil {
					t.Fatal(err)
				}
				if err := L.CallByParam(P{Fn: fn, Protect: true}); err != nil {
					t.Fatal(err)
				}
				if child == nil {
					t.Fatal("coroutine did not execute")
				}
				defer child.Close()
				_, isGrowing := child.stack.(*autoGrowingCallFrameStack)
				if isGrowing != growing {
					t.Fatalf("coroutine stack = %T, growing=%t", child.stack, growing)
				}
				trace := L.GetGlobal("trace").String()
				t.Logf("traceback:\n%s", trace)
				want := "stack traceback:\n\t@traceback.lua:2: in function 'outer'\n\t@traceback.lua:3: in main chunk\n\t[G]: ?"
				if wrapped {
					want = "stack traceback:\n\t@traceback.lua:1: in function 'inner'\n\t@traceback.lua:2: in function 'outer'\n\t@traceback.lua:3: in corountine\n\t[G]: ?"
				}
				if trace != want {
					t.Errorf("traceback = %q, want %q", trace, want)
				}
				var localValue LValue
				for level := 0; ; level++ {
					dbg, ok := child.GetStack(level)
					if !ok {
						break
					}
					if dbg.frame.Fn == nil || dbg.frame.Fn.IsG {
						continue
					}
					for i := 1; ; i++ {
						name, value := child.GetLocal(dbg, i)
						if name == "" {
							break
						}
						if name == "value" {
							localValue = value
						}
					}
				}
				saved := L.GetGlobal("saved").(*LFunction)
				_, captured := L.GetUpvalue(saved, 1)
				if localValue != captured {
					t.Errorf("retained local = %v, captured value = %v", localValue, captured)
				}
				if err := L.DoString(`assert(saved().answer == 42)`); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
