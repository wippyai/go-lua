package lua

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// Expectations come from the Lua 5.4 reference interpreter: results keep the
// integer subtype whenever Lua 5.4 returns an integer, and float results stay
// floats.
func TestIntegerSubtypeResults(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []LValue
	}{
		{"len table", `#{1, 2, 3}`, []LValue{LInteger(3)}},
		{"len empty table", `#{}`, []LValue{LInteger(0)}},
		{"len string", `#"ab"`, []LValue{LInteger(2)}},
		{"len string local", `(function() local s = "abc" return #s end)()`, []LValue{LInteger(3)}},
		{"len __len integer", `#setmetatable({}, {__len = function() return 7 end})`, []LValue{LInteger(7)}},
		{"len __len float", `#setmetatable({}, {__len = function() return 7.0 end})`, []LValue{LNumber(7)}},
		{"len __len string", `#setmetatable({}, {__len = function() return "x" end})`, []LValue{LString("x")}},
		{"select count", `select("#", 1, nil, 3)`, []LValue{LInteger(3)}},
		{"select count empty", `select("#")`, []LValue{LInteger(0)}},

		{"floor float", `math.floor(3.7)`, []LValue{LInteger(3)}},
		{"floor negative", `math.floor(-3.7)`, []LValue{LInteger(-4)}},
		{"floor integer", `math.floor(3)`, []LValue{LInteger(3)}},
		{"floor wide", `math.floor(2^62)`, []LValue{LInteger(1 << 62)}},
		{"floor out of range", `math.floor(1e300)`, []LValue{LNumber(1e300)}},
		{"floor negative zero", `math.floor(-0.0)`, []LValue{LInteger(0)}},
		{"floor numeric string", `math.floor("3.7")`, []LValue{LInteger(3)}},
		{"ceil float", `math.ceil(3.2)`, []LValue{LInteger(4)}},
		{"ceil negative", `math.ceil(-3.2)`, []LValue{LInteger(-3)}},
		{"ceil integer", `math.ceil(3)`, []LValue{LInteger(3)}},
		{"ceil out of range", `math.ceil(1e300)`, []LValue{LNumber(1e300)}},

		{"abs integer", `math.abs(-3)`, []LValue{LInteger(3)}},
		{"abs float", `math.abs(-3.5)`, []LValue{LNumber(3.5)}},
		{"abs mininteger", `math.abs(math.mininteger)`, []LValue{LInteger(math.MinInt64)}},

		{"max integers", `math.max(1, 2)`, []LValue{LInteger(2)}},
		{"max mixed float wins", `math.max(1, 2.0)`, []LValue{LNumber(2)}},
		{"max mixed integer wins", `math.max(1.5, 2)`, []LValue{LInteger(2)}},
		{"max tie keeps first", `math.max(2, 2.0)`, []LValue{LInteger(2)}},
		{"max single", `math.max(3)`, []LValue{LInteger(3)}},
		{"min integers", `math.min(1, 2)`, []LValue{LInteger(1)}},
		{"min mixed float wins", `math.min(1.0, 2)`, []LValue{LNumber(1)}},
		{"min tie keeps first", `math.min(1, 1.0)`, []LValue{LInteger(1)}},

		{"fmod integers", `math.fmod(7, 3)`, []LValue{LInteger(1)}},
		{"fmod negative dividend", `math.fmod(-7, 3)`, []LValue{LInteger(-1)}},
		{"fmod negative divisor", `math.fmod(7, -3)`, []LValue{LInteger(1)}},
		{"fmod mininteger by minus one", `math.fmod(math.mininteger, -1)`, []LValue{LInteger(0)}},
		{"fmod float", `math.fmod(7.0, 3)`, []LValue{LNumber(1)}},
		{"fmod fractional", `math.fmod(5.5, 2)`, []LValue{LNumber(1.5)}},

		{"modf float", `math.modf(3.5)`, []LValue{LInteger(3), LNumber(0.5)}},
		{"modf negative", `math.modf(-3.5)`, []LValue{LInteger(-3), LNumber(-0.5)}},
		{"modf integer", `math.modf(5)`, []LValue{LInteger(5), LNumber(0)}},
		{"modf infinity", `math.modf(math.huge)`, []LValue{LNumber(math.Inf(1)), LNumber(0)}},
		{"modf integral float", `math.modf(4.0)`, []LValue{LInteger(4), LNumber(0)}},

		{"random range", `math.random(3, 3)`, []LValue{LInteger(3)}},
		{"random upper", `math.random(1)`, []LValue{LInteger(1)}},
		{"random full range", `math.type(math.random(math.mininteger, math.maxinteger))`, []LValue{LString("integer")}},
		{"random range type", `math.type(math.random(1, 10))`, []LValue{LString("integer")}},
		{"random zero", `math.type(math.random(0))`, []LValue{LString("integer")}},
		{"random float", `math.type(math.random())`, []LValue{LString("float")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString("return " + tc.expr); err != nil {
				t.Fatal(err)
			}
			if L.GetTop() != len(tc.want) {
				t.Fatalf("got %d results, want %d", L.GetTop(), len(tc.want))
			}
			for i, want := range tc.want {
				got := L.Get(i + 1)
				if got.Type() != want.Type() || got.String() != want.String() {
					t.Fatalf("result %d: got %v (%s), want %v (%s)", i+1, got, got.Type(), want, want.Type())
				}
			}
		})
	}
}

func TestIntegerSubtypeRandomBounds(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
for _ = 1, 200 do
	local x = math.random(-2, 2)
	assert(math.type(x) == "integer" and x >= -2 and x <= 2, tostring(x))
	local y = math.random(5)
	assert(math.type(y) == "integer" and y >= 1 and y <= 5, tostring(y))
end
`); err != nil {
		t.Fatal(err)
	}
}

// Messages follow lmathlib.c.
func TestIntegerSubtypeMathErrors(t *testing.T) {
	cases := []struct {
		expr string
		arg  int
		msg  string
	}{
		{`math.fmod(1, 0)`, 2, "zero"},
		{`math.random(2, 1)`, 1, "interval is empty"},
		{`math.random(0, -1)`, 1, "interval is empty"},
		{`math.random(-1)`, 1, "interval is empty"},
		{`math.random(1.5)`, 1, "number has no integer representation"},
		{`math.random(1, 2.5)`, 2, "number has no integer representation"},
		{`math.random(1, 2, 3)`, 0, "wrong number of arguments"},
		{`math.max()`, 1, "value expected"},
		{`math.min()`, 1, "value expected"},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			err := L.DoString("return " + tc.expr)
			if err == nil {
				t.Fatalf("%s succeeded, want error %q", tc.expr, tc.msg)
			}
			want := tc.msg
			if tc.arg > 0 {
				want = fmt.Sprintf("bad argument #%d to %s (%s)", tc.arg, strings.TrimPrefix(tc.expr[:strings.Index(tc.expr, "(")], "math."), tc.msg)
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%s error %q, want %q", tc.expr, err.Error(), want)
			}
		})
	}
}
