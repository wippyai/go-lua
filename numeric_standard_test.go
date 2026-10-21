package lua

import (
	"fmt"
	"math"
	"testing"
)

// Expectations come from Lua 5.3 sections 3.4.1-3.4.3, not from our VM.
func TestNumericLuaStandard(t *testing.T) {
	cases := []struct {
		a, op, b string
		want     LValue
	}{
		{"7.0", "//", "2.5", LNumber(2)},
		{"7.5", "//", "2.5", LNumber(3)},
		{"-7", "//", "3", LInteger(-3)},
		{"7", "//", "-3", LInteger(-3)},
		{"7.0", "//", "0.0", LNumber(math.Inf(1))},
		{"9007199254740993", "//", "1", LInteger(9007199254740993)},
		{"(0-9223372036854775807-1)", "//", "-1", LInteger(math.MinInt64)},
		{"7", "%", "3", LInteger(1)},
		{"-7", "%", "3", LInteger(2)},
		{"7", "%", "-3", LInteger(-2)},
		{"9007199254740993", "%", "2", LInteger(1)},
		{"(0-9223372036854775807-1)", "%", "-1", LInteger(0)},
		{"7.5", "%", "2.0", LNumber(1.5)},
		{"(0-1)", ">>", "1", LInteger(math.MaxInt64)},
		{"(0-1)", "<<", "-1", LInteger(math.MaxInt64)},
		{"1", ">>", "-63", LInteger(math.MinInt64)},
		{"(0-1)", ">>", "64", LInteger(0)},
		{"1", "<<", "(0-9223372036854775807-1)", LInteger(0)},
		{"3.0", "&", "1", LInteger(1)},
		{"9007199254740993", "&", "1", LInteger(1)},
		{"9223372036854775807", "+", "1", LInteger(math.MinInt64)},
	}
	for _, tc := range cases {
		for _, folded := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s%s%s/folded=%v", tc.a, tc.op, tc.b, folded), func(t *testing.T) {
				L := NewState()
				defer L.Close()
				source := fmt.Sprintf("return %s %s %s", tc.a, tc.op, tc.b)
				if !folded {
					source = fmt.Sprintf("local a,b=%s,%s; return a %s b", tc.a, tc.b, tc.op)
				}
				if err := L.DoString(source); err != nil {
					t.Fatal(err)
				}
				got := L.Get(-1)
				if got != tc.want {
					t.Fatalf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
				}
			})
		}
	}
}

func TestNumericInvalidIntegerConversion(t *testing.T) {
	for _, op := range []string{"&", "|", "~", "<<", ">>"} {
		for _, operand := range []string{"3.5", "9223372036854775808.0", "(1.0/0.0)", "(0.0/0.0)"} {
			for _, folded := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s%s/folded=%v", operand, op, folded), func(t *testing.T) {
					L := NewState()
					defer L.Close()
					source := fmt.Sprintf("return %s %s 1", operand, op)
					if !folded {
						source = fmt.Sprintf("local a=%s; return a %s 1", operand, op)
					}
					fn, err := L.LoadString(source)
					if err != nil {
						t.Fatalf("numeric error must occur during execution: %v", err)
					}
					L.Push(fn)
					if err = L.PCall(0, 1, nil); err == nil {
						t.Fatal("expected integer conversion error")
					}
				})
			}
		}
	}
	for _, source := range []string{"return 1//0", "local a,b=1,0; return a//b", "return 1%0", "local a,b=1,0; return a%b", "return ~3.5"} {
		t.Run(source, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString(source); err == nil {
				t.Fatal("expected numeric error")
			}
		})
	}
}

func TestIntegerUnaryNegationExact(t *testing.T) {
	for _, source := range []string{"return -9007199254740993", "local a=9007199254740993; return -a"} {
		L := NewState()
		if err := L.DoString(source); err != nil {
			L.Close()
			t.Fatal(err)
		}
		got := L.Get(-1)
		L.Close()
		if got != LInteger(-9007199254740993) {
			t.Fatalf("got %v (%T)", got, got)
		}
	}
}

func TestNumericHashLookupDoesNotAllocate(t *testing.T) {
	table := CreateTable(0, 0)
	key := LNumber(1 << 40)
	table.RawSet(key, LTrue)
	for name, lookup := range map[string]func(LValue) LValue{"RawGet": table.RawGet, "RawGetH": table.RawGetH} {
		t.Run(name, func(t *testing.T) {
			var value LValue = key
			if n := testing.AllocsPerRun(1000, func() { numericBenchmarkResult = lookup(value) }); n != 0 {
				t.Fatalf("hash lookup allocated %v times", n)
			}
		})
	}
}

func TestNumericKeyEqualityUsesExactValues(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
  local a,b=9007199254740992,9007199254740993
  assert(a~=b and not rawequal(a,b))
  local f=9007199254740992.0
  assert(a==f and f==a and rawequal(a,f))
  assert(b~=f and f~=b and not rawequal(b,f))
  local max=9223372036854775807
  assert(max~=9223372036854775808.0)
  local t={[a]="a",[b]="b"}
  assert(t[f]=="a" and t[b]=="b")
  assert(0==-0.0 and -0.0==0)
  assert(rawequal(1,1.0) and rawequal(1.0,1))
  local mt={__eq=function() return true end}
  local x,y=setmetatable({},mt),setmetatable({},mt)
  assert(x==y and not rawequal(x,y) and rawequal(x,x))
 `); err != nil {
		t.Fatal(err)
	}
}
