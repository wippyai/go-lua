package lua

import (
	"math"
	"testing"
)

// Lua 5.4 floats keep the sign of zero through every operation that produces
// them; 1/v tells -0.0 (-inf) from 0.0 (+inf).
func TestFloatZeroKeepsSign(t *testing.T) {
	cases := []struct {
		name, expr string
		negative   bool
	}{
		{"len metamethod", "#setmetatable({}, {__len = function() return -0.0 end})", true},
		{"unary minus", "(function() local z = 0.0 return -z end)()", true},
		{"double unary minus", "(function() local z = -0.0 return -(-z) end)()", true},
		{"multiplication", "(function() local z = 0.0 return z * -1 end)()", true},
		{"float multiplication", "(function() local z = 0.0 return -z * 1.0 end)()", true},
		{"division", "(function() local z = 0.0 return z / -1 end)()", true},
		{"underflow", "(function() local x = 2^-1074 return x / -2 end)()", true},
		{"floor division", "(function() local z = -0.0 return z // 1 end)()", true},
		{"modulo", "(function() local z = -0.0 return z % 1 end)()", true},
		{"power", "(function() local z = -0.0 return z ^ 1 end)()", true},
		{"addition", "(function() local z = -0.0 return z + 0 end)()", false},
		{"arith metamethod", "setmetatable({}, {__add = function() return -0.0 end}) + 1", true},
		{"unm metamethod", "-setmetatable({}, {__unm = function() return -0.0 end})", true},
		{"table value", "(function() local t = {} t[1] = -0.0 return t[1] end)()", true},
		{"field value", "(function() local t = {} t.x = -0.0 return t.x end)()", true},
		{"constant", "(function() local a = -0.0 return a end)()", true},
		{"tonumber", "tonumber('-0.0')", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString("local v = " + tc.expr + "\nreturn math.type(v), 1 / v"); err != nil {
				t.Fatal(err)
			}
			if typ := L.Get(-2); typ != LString("float") {
				t.Fatalf("math.type = %v, want float", typ)
			}
			want := LNumber(math.Inf(1))
			if tc.negative {
				want = LNumber(math.Inf(-1))
			}
			if got := L.Get(-1); got != want {
				t.Fatalf("1/v = %v, want %v", got, want)
			}
		})
	}
}

// A -0.0 key is the integer key 0, as for any float key with an integral value.
func TestNegativeZeroTableKey(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local t = {}
		t[-0.0] = "v"
		assert(t[0] == "v" and t[0.0] == "v")
		local k = next(t)
		assert(math.type(k) == "integer" and k == 0)
	`); err != nil {
		t.Fatal(err)
	}
}
