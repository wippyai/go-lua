package lua

import "testing"

// Tail-called Go functions report the call site of the tail call in their
// argument errors, whether the code runs on the main thread, in a coroutine,
// or in a coroutine sliced by the smallest tick budget.
func TestTailCalledGoFunctionErrorPosition(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"string.rep", "return string.rep()",
			"<string>:1: bad argument #1 to rep (string expected, got nil)"},
		{"string.format", `return string.format("%d", 1.5)`,
			"<string>:1: bad argument #2 to format (number has no integer representation)"},
		{"registered", "return check_string()",
			"<string>:1: bad argument #1 to check_string (string expected, got nil)"},
		{"stateless", "return go_check()",
			"<string>:1: bad argument #1 to go_check (string expected, got nil)"},
		{"method", "local s = \"x\"\nreturn s:rep()",
			"<string>:2: bad argument #2 to rep (number expected, got nil)"},
		{"nested", "local function f()\n  return string.rep()\nend\nreturn f()",
			"<string>:2: bad argument #1 to rep (string expected, got nil)"},
		{"nested_method", "local t = {}\nfunction t.run(s)\n  return s:rep({})\nend\nreturn t.run(\"x\")",
			"<string>:3: bad argument #2 to rep (number expected, got table)"},
		{"error_level", "local function f()\n  return error(\"boom\", 2)\nend\nf()",
			"<string>:2: boom"},
	}
	modes := []struct {
		name   string
		budget int64
		thread bool
	}{
		{"main", -1, false},
		{"coroutine", -1, true},
		{"preempted", 1, true},
	}
	for _, tc := range cases {
		for _, mode := range modes {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				L := NewState()
				defer L.Close()
				L.SetGlobal("check_string", L.NewFunction(func(L *LState) int {
					L.CheckString(1)
					return 0
				}))
				L.SetGlobal("go_check", LGoFunc(func(L *LState) int {
					L.CheckString(1)
					return 0
				}))
				src := tc.src
				if mode.budget > 0 {
					// The loop on the first line guarantees preemption before the tail call.
					src = "for _ = 1, 3 do end " + src
				}
				fn, err := L.LoadString(src)
				if err != nil {
					t.Fatal(err)
				}
				if mode.thread {
					th, cancel := L.NewThread()
					defer cancel()
					L.SetTickBudget(mode.budget)
					preempts := 0
					var st ResumeState
					for st, _, err = L.Resume(th, fn); st == ResumePreempted; st, _, err = L.Resume(th, fn) {
						preempts++
						L.SetTickBudget(mode.budget)
					}
					if mode.budget > 0 && preempts == 0 {
						t.Fatal("expected preemption")
					}
				} else {
					L.Push(fn)
					err = L.PCall(0, 0, nil)
				}
				apiErr, ok := err.(*ApiError)
				if !ok {
					t.Fatalf("error = %v, want %q", err, tc.want)
				}
				if got := apiErr.Object.String(); got != tc.want {
					t.Fatalf("error = %q, want %q", got, tc.want)
				}
			})
		}
	}
}
