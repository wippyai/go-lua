package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/parse"
)

// Lua 5.1 and 5.4 evaluate every RHS before changing any assignment target.
func TestMultipleAssignmentSnapshotsRightHandValues(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   []LValue
	}{
		{"local swap", `local a, b = 1, 2; a, b = b, a; return a, b`, []LValue{LNumber(2), LNumber(1)}},
		{"local expressions", `local a, b = 1, 2; a, b = b + 0, a + 0; return a, b`, []LValue{LNumber(2), LNumber(1)}},
		{"three local cycle", `local a, b, c = 1, 2, 3; a, b, c = c, a, b; return a, b, c`, []LValue{LNumber(3), LNumber(1), LNumber(2)}},
		{"repeated local target", `local a = 0; a, a = 1, 2; return a`, []LValue{LNumber(1)}},
		{"upvalue swap", `local a, b = 1, 2; local function swap() a, b = b, a end; swap(); return a, b`, []LValue{LNumber(2), LNumber(1)}},
		{"mixed local and upvalue", `local outer = 1; local function swap() local inner = 2; inner, outer = outer, inner; return inner, outer end; return swap()`, []LValue{LNumber(1), LNumber(2)}},
		{"global swap", `ga, gb = 1, 2; ga, gb = gb, ga; return ga, gb`, []LValue{LNumber(2), LNumber(1)}},
		{"mixed local and global", `local a = 1; gb = 2; a, gb = gb, a; return a, gb`, []LValue{LNumber(2), LNumber(1)}},
		{"mixed local and table", `local a = 1; local t = {x = 2}; a, t.x = t.x, a; return a, t.x`, []LValue{LNumber(2), LNumber(1)}},
		{"table then local", `local a = 1; local t = {x = 2}; t.x, a = a, t.x; return a, t.x`, []LValue{LNumber(2), LNumber(1)}},
		{"table target key before writes", `local i = 1; local t = {10, 20}; i, t[i] = 2, i + 10; return i, t[1], t[2]`, []LValue{LNumber(2), LNumber(11), LNumber(20)}},
		{"table target object before writes", `local old = {x = 0}; local t = old; t.x, t = 1, {x = 2}; return old.x, t.x`, []LValue{LNumber(1), LNumber(2)}},
		{"local before table target object", `local old = {x = 0}; local t = old; t, t.x = {x = 2}, 1; return old.x, t.x`, []LValue{LNumber(1), LNumber(2)}},
		{"single table target object before call", `local old = {x = 0}; local t = old; local function switch() t = {x = 2}; return 1 end; t.x = switch(); return old.x, t.x`, []LValue{LNumber(0), LNumber(1)}},
		{"table target object changed by RHS call", `local old = {x = 0}; local t = old; local y = 0; local function switch() t = {x = 2}; return 1 end; t.x, y = switch(), 0; return old.x, t.x, y`, []LValue{LNumber(0), LNumber(1), LNumber(0)}},
		{"table field swap", `local t = {x = 1, y = 2}; t.x, t.y = t.y, t.x; return t.x, t.y`, []LValue{LNumber(2), LNumber(1)}},
		{"vararg results", `local function f(...) local a, b = 1, 2; a, b = ...; return a, b end; return f(8, 9)`, []LValue{LNumber(8), LNumber(9)}},
		{"vararg missing result", `local function f(...) local a, b = 1, 2; a, b = ...; return a, b end; return f(8)`, []LValue{LNumber(8), LNil}},
		{"call results", `local a, b = 1, 2; local function pair() return b, a end; a, b = pair(); return a, b`, []LValue{LNumber(2), LNumber(1)}},
		{"call reads old local", `local a, b = 1, 2; local function first(x) return x end; a, b = b, first(a); return a, b`, []LValue{LNumber(2), LNumber(1)}},
		{"call expands after local", `local a, b, c = 1, 2, 3; local function pair(x, y) return x, y end; a, b, c = c, pair(a, b); return a, b, c`, []LValue{LNumber(3), LNumber(1), LNumber(2)}},
		{"extra RHS reads old locals", `local a, b, seen = 1, 2, 0; local function observe() seen = a * 10 + b end; a, b = b, a, observe(); return a, b, seen`, []LValue{LNumber(2), LNumber(1), LNumber(12)}},
		{"single local extra RHS reads old local", `local a, seen = 1, 0; local function observe() seen = a end; a = 2, observe(); return a, seen`, []LValue{LNumber(2), LNumber(1)}},
		{"nil padded", `local a, b = 1, 2; a, b = b; return a, b`, []LValue{LNumber(2), LNil}},
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
		{"CompileString/LoadProto", func(t *testing.T, L *LState, source string) *LFunction {
			proto, err := CompileString(source, "multiassign.lua")
			if err != nil {
				t.Fatal(err)
			}
			return L.LoadProto(proto)
		}},
		{"CompileWithOptions/LoadProto", func(t *testing.T, L *LState, source string) *LFunction {
			chunk, err := parse.ParseString(source, "multiassign.lua")
			if err != nil {
				t.Fatal(err)
			}
			proto, err := CompileWithOptions(chunk, "multiassign.lua", CompileOptions{})
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
					if err := L.PCall(0, len(tc.want), nil); err != nil {
						t.Fatal(err)
					}
					for i, want := range tc.want {
						got := L.Get(i - len(tc.want))
						equal := got == want
						if _, numeric := want.(LNumber); numeric {
							switch got.(type) {
							case LNumber, LInteger:
								equal = LVAsNumber(got) == LVAsNumber(want)
							}
						}
						if !equal {
							t.Errorf("result %d: got %v, want %v", i+1, got, want)
						}
					}
				})
			}
		})
	}
}
