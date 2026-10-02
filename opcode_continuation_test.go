package lua

import (
	"fmt"
	"strings"
	"testing"
)

// runToCompletion resumes src until it finishes, resuming through yields and,
// when budget >= 0, through preemptions. It returns the results and the number
// of yields and preemptions seen.
func runToCompletion(t *testing.T, L *LState, src string, budget int64) (ret []LValue, yields, preempts int) {
	t.Helper()
	fn, err := L.LoadString(src)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	co, cancel := L.NewThread()
	defer cancel()
	for i := 0; i < 1_000_000; i++ {
		L.SetTickBudget(budget)
		st, res, err := L.Resume(co, fn)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		switch st {
		case ResumeYield:
			yields++
		case ResumePreempted:
			preempts++
		case ResumeOK:
			return res, yields, preempts
		}
	}
	t.Fatal("no progress")
	return nil, 0, 0
}

func expectString(t *testing.T, got []LValue, want string) {
	t.Helper()
	if len(got) != 1 || got[0] != LString(want) {
		t.Fatalf("expected %q, got %v", want, got)
	}
}

func sprintfBody(src, body string) string { return fmt.Sprintf(src, body) }

const slowBody = `local s = 0 for i = 1, 50 do s = s + i end`

func TestConcatContinuationKeepsOperands(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"middle", `local v = setmetatable({}, {__concat = function(a, b) %s return (type(a) == "table" and "J" or a) .. (type(b) == "table" and "J" or b) end})
return "prefix" .. v .. "suffix"`, "prefixJsuffix"},
		{"last_operand", `local v = setmetatable({}, {__concat = function(a, b) %s return (type(a) == "table" and "J" or a) .. (type(b) == "table" and "J" or b) end})
return "a" .. "b" .. v`, "abJ"},
		{"first_operand", `local v = setmetatable({}, {__concat = function(a, b) %s return (type(a) == "table" and "J" or a) .. (type(b) == "table" and "J" or b) end})
return v .. "x" .. "y"`, "Jxy"},
		{"two_metamethod_operands", `local mt = {__concat = function(a, b) %s
	local x = type(a) == "table" and a.n or a
	local y = type(b) == "table" and b.n or b
	return x .. y end}
local p, q = setmetatable({n = "P"}, mt), setmetatable({n = "Q"}, mt)
return "1" .. p .. "2" .. q .. "3"`, "1P2Q3"},
		{"four_operands", `local v = setmetatable({}, {__concat = function(a, b) %s return (type(a) == "table" and "J" or a) .. (type(b) == "table" and "J" or b) end})
return "a" .. "b" .. v .. "c" .. "d"`, "abJcd"},
	}
	for _, c := range cases {
		for _, mode := range []string{"yield", "preempt"} {
			t.Run(c.name+"_"+mode, func(t *testing.T) {
				L := NewState()
				defer L.Close()
				body, budget := "coroutine.yield(1)", int64(-1)
				if mode == "preempt" {
					body, budget = slowBody, 3
				}
				src := sprintfBody(c.src, body)
				ret, yields, preempts := runToCompletion(t, L, src, budget)
				if yields+preempts == 0 {
					t.Fatal("expected a suspension")
				}
				expectString(t, ret, c.want)
			})
		}
	}
}

func TestLessEqualViaLessThanKeepsInversion(t *testing.T) {
	cases := []struct {
		name, expr, want string
	}{
		{"le_true", "a <= b", "true"},
		{"le_false", "b <= a", "false"},
		{"le_equal", "a <= a", "true"},
		{"ge_true", "b >= a", "true"},
	}
	for _, c := range cases {
		for _, mode := range []string{"yield", "preempt"} {
			t.Run(c.name+"_"+mode, func(t *testing.T) {
				L := NewState()
				defer L.Close()
				body, budget := "coroutine.yield(1)", int64(-1)
				if mode == "preempt" {
					body, budget = slowBody, 3
				}
				src := sprintfBody(`local mt = {__lt = function(x, y) %s return x.n < y.n end}
local a, b = setmetatable({n = 1}, mt), setmetatable({n = 2}, mt)
local r = `+c.expr+`
return tostring(r)`, body)
				ret, yields, preempts := runToCompletion(t, L, src, budget)
				if yields+preempts == 0 {
					t.Fatal("expected a suspension")
				}
				expectString(t, ret, c.want)
			})
		}
	}
}

func TestPcallDoesNotInheritCompletedXpcallHandler(t *testing.T) {
	for _, mode := range []string{"yield", "preempt"} {
		t.Run(mode, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			body, budget := "coroutine.yield(1)", int64(-1)
			if mode == "preempt" {
				body, budget = slowBody, 3
			}
			src := sprintfBody(`
local ok1 = xpcall(function() %[1]s return 1 end, function(e) return "OLD" end)
assert(ok1)
local ok, err = pcall(function() %[1]s error("boom") end)
return tostring(ok) .. ":" .. tostring(err)`, body)
			ret, yields, preempts := runToCompletion(t, L, src, budget)
			if yields+preempts == 0 {
				t.Fatal("expected a suspension")
			}
			if len(ret) != 1 || !strings.Contains(string(ret[0].(LString)), "boom") || strings.Contains(string(ret[0].(LString)), "OLD") {
				t.Fatalf("unexpected result %v", ret)
			}
		})
	}
}
