package lua

import (
	"math"
	"strings"
	"testing"
)

// Expectations come from the Lua 5.4 reference interpreter.
func TestStringToNumberConversion(t *testing.T) {
	inf := LNumber(math.Inf(1))
	cases := []struct {
		name string
		expr string
		want LValue
	}{
		{"tonumber integer", `tonumber("10")`, LInteger(10)},
		{"tonumber signed", `tonumber("+5")`, LInteger(5)},
		{"tonumber negative zero integer", `tonumber("-0")`, LInteger(0)},
		{"tonumber whitespace", `tonumber("\t\v\f\r 5 \n")`, LInteger(5)},
		{"tonumber hex", `tonumber("0x10")`, LInteger(16)},
		{"tonumber signed hex", `tonumber("+0x5")`, LInteger(5)},
		{"tonumber hex wraps", `tonumber("0xffffffffffffffffff")`, LInteger(-1)},
		{"tonumber exponent", `tonumber("1e5")`, LNumber(1e5)},
		{"tonumber fraction", `tonumber("5.")`, LNumber(5)},
		{"tonumber integer overflow", `tonumber("9223372036854775808")`, LNumber(9223372036854775808)},
		{"tonumber min integer", `tonumber("-9223372036854775808")`, LInteger(math.MinInt64)},
		{"tonumber float overflow", `tonumber("1e400")`, inf},
		{"tonumber float underflow", `tonumber("1e-400")`, LNumber(0)},
		{"tonumber hex float", `tonumber("0x1p4")`, LNumber(16)},
		{"tonumber hex fraction", `tonumber(" 0x1.8 ")`, LNumber(1.5)},
		{"tonumber hex denormal", `tonumber("0x1P-1074")`, LNumber(5e-324)},
		{"tonumber inf", `tonumber("inf")`, LNil},
		{"tonumber nan", `tonumber("nan")`, LNil},
		{"tonumber empty", `tonumber("")`, LNil},
		{"tonumber dangling exponent", `tonumber("1e")`, LNil},
		{"tonumber bare hex prefix", `tonumber("0x")`, LNil},
		{"tonumber underscore", `tonumber("0x1_0")`, LNil},
		{"tonumber embedded zero", `tonumber("1\0")`, LNil},
		{"tonumber integer argument", `tonumber(7)`, LInteger(7)},
		{"tonumber float argument", `tonumber(7.5)`, LNumber(7.5)},
		{"tonumber non-number", `tonumber({})`, LNil},
		{"tonumber base 16", `tonumber("ff", 16)`, LInteger(255)},
		{"tonumber base 36", `tonumber("z", 36)`, LInteger(35)},
		{"tonumber base 2 whitespace", `tonumber(" 10 ", 2)`, LInteger(2)},
		{"tonumber base negative", `tonumber("-ff", 16)`, LInteger(-255)},
		{"tonumber base max integer", `tonumber("7fffffffffffffff", 16)`, LInteger(math.MaxInt64)},
		{"tonumber base wraps", `tonumber("ffffffffffffffff", 16)`, LInteger(-1)},
		{"tonumber base rejects exponent", `tonumber("1e1", 10)`, LNil},
		{"tonumber base rejects prefix", `tonumber("0x10", 16)`, LNil},
		{"tonumber base rejects digit", `tonumber("2", 2)`, LNil},
		{"add integer string", `"10" + 0`, LInteger(10)},
		{"add float string", `"1e2" + 0`, LNumber(100)},
		{"mul hex string", `"0x10" * 1`, LInteger(16)},
		{"sub strings", `"10" - "3"`, LInteger(7)},
		{"mod string", `"7" % 3`, LInteger(1)},
		{"div string", `"7" / 2`, LNumber(3.5)},
		{"pow string", `"2" ^ 2`, LNumber(4)},
		{"idiv string", `" 7 " // 2`, LInteger(3)},
		{"idiv float string", `"7.0" // 2`, LNumber(3)},
		{"unary minus string", `-"10"`, LInteger(-10)},
		{"unary minus float string", `-"1.5"`, LNumber(-1.5)},
		{"band string", `" 3 " & 1`, LInteger(1)},
		{"string overflow to float", `"9223372036854775808" + 0`, LNumber(9223372036854775808)},
		{"math.tointeger string", `math.tointeger("8")`, LInteger(8)},
		{"math.tointeger float string", `math.tointeger("8.0")`, LInteger(8)},
		{"math.tointeger fractional string", `math.tointeger("8.5")`, LNil},
		{"math.tointeger out of range", `math.tointeger(2.0^63)`, LNil},
		{"literal hex wraps", `0xffffffffffffffff`, LInteger(-1)},
		{"literal hex float", `0x1p4`, LNumber(16)},
		{"literal hex fraction", `0xA.8p0`, LNumber(10.5)},
		{"literal hex leading dot", `0x.8`, LNumber(0.5)},
		{"literal float overflow", `1e400`, inf},
		{"literal integer overflow", `9223372036854775808`, LNumber(9223372036854775808)},
		{"literal leading zero", `010`, LInteger(10)},
		{"literal trailing dot", `3.`, LNumber(3)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString("return " + tc.expr); err != nil {
				t.Fatal(err)
			}
			got := L.Get(-1)
			if got != tc.want {
				t.Fatalf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

func TestStringToNumberErrors(t *testing.T) {
	cases := []struct {
		name, source, want string
	}{
		{"tonumber base out of range", `return tonumber("10", 99)`, "base out of range"},
		{"tonumber base needs string", `return tonumber(10, 16)`, "string expected, got number"},
		{"malformed literal", `return 3x`, "malformed number"},
		{"malformed hex literal", `return 0x`, "malformed number"},
		{"malformed exponent literal", `return 1e`, "malformed number"},
		{"malformed double dot literal", `return 1.2.3`, "malformed number"},
		{"integer modulo by zero string", `return "7" % "0"`, "modulo by zero"},
		{"integer floor division by zero string", `return "7" // "0"`, "divide by zero"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			err := L.DoString(tc.source)
			if err == nil {
				t.Fatalf("expected error containing %q, got %v", tc.want, L.Get(-1))
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}
