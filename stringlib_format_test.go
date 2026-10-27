package lua

import (
	"strings"
	"testing"
)

// Expectations are the output of the Lua 5.4 reference interpreter.
func TestStringFormatLuaStandard(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`string.format("%.1f", 0)`, "0.0"},
		{`string.format("%5.1f%%", 25)`, " 25.0%"},
		{`string.format("%f", 3)`, "3.000000"},
		{`string.format("%e", 7)`, "7.000000e+00"},
		{`string.format("%g", 1/3)`, "0.333333"},
		{`string.format("%g", 100000000000000)`, "1e+14"},
		{`string.format("%.3g", 2)`, "2"},
		{`string.format("%10.2f|", 3.14159)`, "      3.14|"},
		{`string.format("%-8.2f|", -2)`, "-2.00   |"},
		{`string.format("%+.0f", 2.5)`, "+2"},
		{`string.format("%f", 1/0)`, "inf"},
		{`string.format("%5.1f", -1/0)`, " -inf"},
		{`string.format("%.2f", "1.5")`, "1.50"},
		{`string.format("%d", 3.0)`, "3"},
		{`string.format("%5d|", 42)`, "   42|"},
		{`string.format("%-5d|", 42)`, "42   |"},
		{`string.format("%05d", -42)`, "-0042"},
		{`string.format("%.3d", 7)`, "007"},
		{`string.format("%+d % d", 5, 5)`, "+5  5"},
		{`string.format("%d", "10")`, "10"},
		{`string.format("%i", -7)`, "-7"},
		{`string.format("%x %X %o", 255, 255, 8)`, "ff FF 10"},
		{`string.format("%#x %#o", 255, 8)`, "0xff 010"},
		{`string.format("%x", -1)`, "ffffffffffffffff"},
		{`string.format("%c%c%c", 76, 117, 97)`, "Lua"},
		{`string.format("%3c|", 65)`, "  A|"},
		{`string.format("%s", 1.5)`, "1.5"},
		{`string.format("%s", 10)`, "10"},
		{`string.format("%s", nil)`, "nil"},
		{`string.format("%s", true)`, "true"},
		{`string.format("%.3s|", "h\195\169llo")`, "hé|"},
		{`string.format("%6s|", "\195\169")`, "    é|"},
		{`string.format("%-6s|", "ab")`, "ab    |"},
		{`string.format("%5.2s|", "abc")`, "   ab|"},
		{`string.format("%s", setmetatable({}, {__tostring = function() return "obj" end}))`, "obj"},
		{`string.format("%q", "a\nb\"c\\\0d\1e\200")`, "\"a\\\nb\\\"c\\\\\\0d\\1e\xc8\""},
		{`string.format("%q", "\0001")`, `"\0001"`},
		{`string.format("%q", "\r")`, `"\13"`},
		{`string.format("%q", 42)`, "42"},
		{`string.format("%q", 1.5)`, "0x1.8p+0"},
		{`string.format("%q", 2.0)`, "0x1p+1"},
		{`string.format("%q", math.mininteger)`, "0x8000000000000000"},
		{`string.format("%q", 1/0)`, "1e9999"},
		{`string.format("%q", -1/0)`, "-1e9999"},
		{`string.format("%q", 0/0)`, "(0/0)"},
		{`string.format("%q", nil)`, "nil"},
		{`string.format("%q", false)`, "false"},
		{`string.format("%a", 1)`, "0x1p+0"},
		{`string.format("%A", 0.5)`, "0X1P-1"},
		{`string.format("%.2a", 1/3)`, "0x1.55p-2"},
		{`string.format("%a", 0)`, "0x0p+0"},
		{`string.format("%a", 1/0)`, "inf"},
		{`string.format("100%%")`, "100%"},
		{`string.format("%s-%s", "a", "b")`, "a-b"},
		{`string.format("%G %E", 1e-10, 12345.678)`, "1E-10 1.234568E+04"},
		{`string.format("%#g", 1)`, "1.00000"},
		{`string.format("%.0e", 12345)`, "1e+04"},
		{`string.format("%d", math.mininteger)`, "-9223372036854775808"},
		{`string.format("%.14g", 0.1)`, "0.1"},
		{`string.format("%F", 1/0)`, "INF"},
		{`string.format("%s", "a", "extra")`, "a"},
		{`string.format("%+.0d", 0)`, "+"},
		{`string.format("% .0d", 0)`, " "},
		{`string.format("%#.0o", 0)`, "0"},
		{`string.format("%#x", 0)`, "0"},
		{`string.format("%.0x", 0)`, ""},
		{`string.format("%+5.0d", -0.0)`, "    +"},
		{`string.format("%#5X", 255)`, " 0XFF"},
		{`string.format("%-#8.3o|", 8)`, "010     |"},
		{`string.format("%08.3d", 5)`, "     005"},
		{`string.format("%u", -1)`, "18446744073709551615"},
		{`string.format("%+08.2f", -1.5)`, "-0001.50"},
		{`string.format("%010.3a", 1)`, "0x1.000p+0"},
		{`string.format("%-+8a|", -0.5)`, "-0x1p-1 |"},
		{`string.format("%05f", 1/0)`, "  inf"},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString("return " + tc.expr); err != nil {
				t.Fatal(err)
			}
			got, ok := L.Get(-1).(LString)
			if !ok {
				t.Fatalf("got %T, want string", L.Get(-1))
			}
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStringFormatLuaStandardErrors(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`string.format("%d", 1.5)`, "bad argument #2 to format (number has no integer representation)"},
		{`string.format("%d", 2^63)`, "bad argument #2 to format (number has no integer representation)"},
		{`string.format("%d", "1.5")`, "bad argument #2 to format (number has no integer representation)"},
		{`string.format("%c", 1.5)`, "bad argument #2 to format (number has no integer representation)"},
		{`string.format("%d")`, "bad argument #2 to format (no value)"},
		{`string.format("%s %s", 1)`, "bad argument #3 to format (no value)"},
		{`string.format("%d", "x")`, "bad argument #2 to format (number expected, got string)"},
		{`string.format("%f", "x")`, "bad argument #2 to format (number expected, got string)"},
		{`string.format("%q", {})`, "bad argument #2 to format (value has no literal form)"},
		{`string.format("%y", 1)`, "invalid conversion '%y' to 'format'"},
		{`string.format("%", 1)`, "invalid conversion '%' to 'format'"},
		{`string.format("%10q", "a")`, "specifier '%q' cannot have modifiers"},
		{`string.format("%100d", 1)`, "invalid conversion specification: '%100d'"},
		{`string.format("%#d", 1)`, "invalid conversion specification: '%#d'"},
		{`string.format("%.3c", 65)`, "invalid conversion specification: '%.3c'"},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			err := L.DoString("return " + tc.expr)
			if err == nil {
				t.Fatalf("got %v, want error %q", L.Get(-1), tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}
