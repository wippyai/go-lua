package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/parse"
)

// A call whose result is assigned to a parameter reads that parameter's old
// value as its callee, receiver, or argument.
func TestCallAssignedToParameterReadsOldParameter(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   LValue
	}{
		{"argument", `local function trim(v) return v:match("^%s*(.-)%s*$") end
local function f(raw) raw = trim(raw); return raw end
return f("  a ")`, LString("a")},
		{"parenthesized argument", `local function trim(v) return v:match("^%s*(.-)%s*$") end
local function f(raw) raw = (trim(raw)); return raw end
return f("  a ")`, LString("a")},
		{"method receiver", `local function f(raw) raw = raw:upper(); return raw end
return f("a")`, LString("A")},
		{"parenthesized method receiver", `local function f(raw) raw = (raw:upper()); return raw end
return f("a")`, LString("A")},
		{"last of several parameters", `local function f(a, raw) raw = a .. raw:upper(); raw = string.rep(raw, 2); return raw end
return f("x", "y")`, LString("xYxY")},
		{"callee", `local function f(fn) fn = fn(); return fn end
return f(function() return "called" end)`, LString("called")},
		{"typed library function", `local M = {}
local function trim(value: string): string
    return value:match("^%s*(.-)%s*$") or ""
end
local function parse_set(raw: string): string
    raw = trim(raw)
    return raw
end
function M.parse(raw: string): string
    return parse_set(raw)
end
return M.parse("  1.2.3 ")`, LString("1.2.3")},
	}
	paths := []struct {
		name string
		load func(*testing.T, *LState, string) *LFunction
	}{
		{"LoadString", func(t *testing.T, L *LState, source string) *LFunction {
			fn, err := L.LoadString(source)
			if err != nil {
				t.Fatal(err)
			}
			return fn
		}},
		{"CompileWithOptions/LoadProto", func(t *testing.T, L *LState, source string) *LFunction {
			chunk, err := parse.ParseString(source, "call_parameter_target.lua")
			if err != nil {
				t.Fatal(err)
			}
			proto, err := CompileWithOptions(chunk, "call_parameter_target.lua", CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			return L.LoadProto(proto)
		}},
	}
	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					L := NewState()
					defer L.Close()
					L.Push(path.load(t, L, tc.source))
					if err := L.PCall(0, 1, nil); err != nil {
						t.Fatal(err)
					}
					if got := L.Get(-1); got != tc.want {
						t.Errorf("got %v, want %v", got, tc.want)
					}
				})
			}
		})
	}
}
