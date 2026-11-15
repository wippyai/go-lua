package lua

import (
	"strings"
	"testing"
)

const utf8Hallo = "H\xC3\xA4llo"

// Expectations come from the Lua 5.4 reference interpreter.
func TestUtf8Lib(t *testing.T) {
	cases := []struct {
		name string
		s    string
		expr string
		want []LValue
	}{
		{"char", "", `utf8.char(72, 228, 8364, 128512)`, []LValue{LString("H\xC3\xA4\xE2\x82\xAC\xF0\x9F\x98\x80")}},
		{"char none", "", `utf8.char()`, []LValue{LString("")}},
		{"char surrogate", "", `utf8.char(0xD800)`, []LValue{LString("\xED\xA0\x80")}},
		{"char beyond unicode", "", `utf8.char(0x110000)`, []LValue{LString("\xF4\x90\x80\x80")}},
		{"char max", "", `utf8.char(0x7FFFFFFF)`, []LValue{LString("\xFD\xBF\xBF\xBF\xBF\xBF")}},
		{"charpattern", "", `utf8.charpattern`, []LValue{LString("[\x00-\x7F\xC2-\xFD][\x80-\xBF]*")}},

		{"len", utf8Hallo, `utf8.len(s)`, []LValue{LInteger(5)}},
		{"len continuation start", utf8Hallo, `utf8.len(s, 3)`, []LValue{LNil, LInteger(3)}},
		{"len negative start", utf8Hallo, `utf8.len(s, -2)`, []LValue{LInteger(2)}},
		{"len negative end", utf8Hallo, `utf8.len(s, 1, -3)`, []LValue{LInteger(3)}},
		{"len empty", "", `utf8.len(s)`, []LValue{LInteger(0)}},
		{"len start past end", "abc", `utf8.len(s, 4)`, []LValue{LInteger(0)}},
		{"len invalid byte", "a\xFFb", `utf8.len(s)`, []LValue{LNil, LInteger(2)}},
		{"len truncated", "a\xC3", `utf8.len(s)`, []LValue{LNil, LInteger(2)}},
		{"len surrogate", "\xED\xA0\x80", `utf8.len(s)`, []LValue{LNil, LInteger(1)}},
		{"len surrogate lax", "\xED\xA0\x80", `utf8.len(s, 1, -1, true)`, []LValue{LInteger(1)}},
		{"len beyond unicode", "\xF4\x90\x80\x80", `utf8.len(s)`, []LValue{LNil, LInteger(1)}},
		{"len beyond unicode lax", "\xF4\x90\x80\x80", `utf8.len(s, 1, -1, true)`, []LValue{LInteger(1)}},
		{"len overlong", "\xC0\x80", `utf8.len(s)`, []LValue{LNil, LInteger(1)}},

		{"codepoint", utf8Hallo, `utf8.codepoint(s)`, []LValue{LInteger(72)}},
		{"codepoint range", utf8Hallo, `utf8.codepoint(s, 1, -1)`, []LValue{LInteger(72), LInteger(228), LInteger(108), LInteger(108), LInteger(111)}},
		{"codepoint negative", utf8Hallo, `utf8.codepoint(s, -1)`, []LValue{LInteger(111)}},
		{"codepoint multibyte", utf8Hallo, `utf8.codepoint(s, 2)`, []LValue{LInteger(228)}},
		{"codepoint empty range", "abc", `utf8.codepoint(s, 3, 2)`, []LValue{}},
		{"codepoint surrogate lax", "\xED\xA0\x80", `utf8.codepoint(s, 1, 1, true)`, []LValue{LInteger(0xD800)}},

		{"offset", utf8Hallo, `utf8.offset(s, 3)`, []LValue{LInteger(4)}},
		{"offset last", utf8Hallo, `utf8.offset(s, -1)`, []LValue{LInteger(6)}},
		{"offset second last", utf8Hallo, `utf8.offset(s, -2)`, []LValue{LInteger(5)}},
		{"offset zero", utf8Hallo, `utf8.offset(s, 0, 3)`, []LValue{LInteger(2)}},
		{"offset from position", utf8Hallo, `utf8.offset(s, 2, 2)`, []LValue{LInteger(4)}},
		{"offset end", utf8Hallo, `utf8.offset(s, 6)`, []LValue{LInteger(7)}},
		{"offset past end", utf8Hallo, `utf8.offset(s, 7)`, []LValue{LNil}},
		{"offset before start", utf8Hallo, `utf8.offset(s, -6)`, []LValue{LNil}},
		{"offset zero at end", "abc", `utf8.offset(s, 0, 4)`, []LValue{LInteger(4)}},
		{"offset empty", "", `utf8.offset(s, 1)`, []LValue{LInteger(1)}},
		{"offset empty negative", "", `utf8.offset(s, -1)`, []LValue{LNil}},

		{"codes", "H\xC3\xA4\xE2\x82\xAC!", `(function() local r = {} for p, c in utf8.codes(s) do r[#r+1] = p .. ":" .. c end return table.concat(r, " ") end)()`, []LValue{LString("1:72 2:228 4:8364 7:33")}},
		{"codes empty", "", `(function() local n = 0 for _ in utf8.codes(s) do n = n + 1 end return n end)()`, []LValue{LInteger(0)}},
		{"codes surrogate lax", "\xED\xA0\x80", `(function() for _, c in utf8.codes(s, true) do return c end end)()`, []LValue{LInteger(0xD800)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetGlobal("s", LString(tc.s))
			if err := L.DoString("return " + tc.expr); err != nil {
				t.Fatal(err)
			}
			if L.GetTop() != len(tc.want) {
				t.Fatalf("got %d results, want %d", L.GetTop(), len(tc.want))
			}
			for i, want := range tc.want {
				if got := L.Get(i + 1); got != want {
					t.Fatalf("result %d: got %q (%T), want %q (%T)", i+1, got.String(), got, want.String(), want)
				}
			}
		})
	}
}

func TestUtf8LibErrors(t *testing.T) {
	cases := []struct {
		name, s, expr, want string
	}{
		{"char negative", "", `utf8.char(-1)`, "value out of range"},
		{"char too large", "", `utf8.char(0x80000000)`, "value out of range"},
		{"len start past end", "abc", `utf8.len(s, 5)`, "initial position out of bounds"},
		{"len start zero", "abc", `utf8.len(s, 0)`, "initial position out of bounds"},
		{"len end past end", "abc", `utf8.len(s, 1, 4)`, "final position out of bounds"},
		{"codepoint continuation", utf8Hallo, `utf8.codepoint(s, 3)`, "invalid UTF-8 code"},
		{"codepoint start zero", "abc", `utf8.codepoint(s, 0)`, "out of bounds"},
		{"codepoint end past end", "abc", `utf8.codepoint(s, 1, 4)`, "out of bounds"},
		{"codepoint empty", "", `utf8.codepoint(s)`, "out of bounds"},
		{"codepoint surrogate", "\xED\xA0\x80", `utf8.codepoint(s)`, "invalid UTF-8 code"},
		{"offset continuation", utf8Hallo, `utf8.offset(s, 1, 3)`, "initial position is a continuation byte"},
		{"offset position past end", "abc", `utf8.offset(s, 1, 5)`, "position out of bounds"},
		{"offset position zero", "abc", `utf8.offset(s, 1, 0)`, "position out of bounds"},
		{"codes invalid byte", "a\xFFb", `(function() for _ in utf8.codes(s) do end end)()`, "invalid UTF-8 code"},
		{"codes truncated", "a\xC3", `(function() for _ in utf8.codes(s) do end end)()`, "invalid UTF-8 code"},
		{"codes surrogate", "\xED\xA0\x80", `(function() for _ in utf8.codes(s) do end end)()`, "invalid UTF-8 code"},
		{"codes leading continuation", "\x80", `(function() for _ in utf8.codes(s) do end end)()`, "invalid UTF-8 code"},
		{"codes stray continuation", "\xC3\xA4\x80", `(function() for _ in utf8.codes(s) do end end)()`, "invalid UTF-8 code"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetGlobal("s", LString(tc.s))
			err := L.DoString("return " + tc.expr)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}
