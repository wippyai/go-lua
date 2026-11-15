package numparse

import (
	"math"
	"strconv"
	"testing"
)

func TestParseNonNegativeDecimalInt(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := strconv.FormatInt(int64(maxInt), 10) + "0"

	tests := []struct {
		input string
		want  int
		ok    bool
	}{
		{"0", 0, true},
		{"42", 42, true},
		{"001", 1, true},
		{"", 0, false},
		{"-1", 0, false},
		{"+1", 0, false},
		{"1e3", 0, false},
		{"abc", 0, false},
		{overflow, 0, false},
	}

	for _, tt := range tests {
		got, ok := ParseNonNegativeDecimalInt(tt.input)
		if ok != tt.ok {
			t.Fatalf("ParseNonNegativeDecimalInt(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseNonNegativeDecimalInt(%q)=%d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseIntegerLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"42", 42, true},
		{"08", 8, true},
		{"0xDEAD", 0xDEAD, true},
		{"0x1p2", 0, false},
		{"1e3", 0, false},
		{"0.0", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseIntegerLiteral(tt.input)
		if ok != tt.ok {
			t.Fatalf("ParseIntegerLiteral(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseIntegerLiteral(%q)=%d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseNumberLiteral(t *testing.T) {
	tests := []struct {
		input   string
		wantInt int64
		wantFlt float64
		isInt   bool
		parseOK bool
	}{
		{"08", 8, 0, true, true},
		{"0xDEAD", 0xDEAD, 0, true, true},
		{"3.14", 0, 3.14, false, true},
		{"0x1p2", 0, 4, false, true},
		{"nope", 0, 0, false, false},
	}
	for _, tt := range tests {
		i, f, ok := ParseNumberLiteral(tt.input)
		if ok != tt.parseOK {
			t.Fatalf("ParseNumberLiteral(%q) ok=%v, want %v", tt.input, ok, tt.parseOK)
		}
		if !ok {
			continue
		}
		if tt.isInt {
			if i != tt.wantInt || f != 0 {
				t.Fatalf("ParseNumberLiteral(%q) got int=%d float=%f, want int=%d float=0", tt.input, i, f, tt.wantInt)
			}
			continue
		}
		if i != 0 || f != tt.wantFlt {
			t.Fatalf("ParseNumberLiteral(%q) got int=%d float=%f, want int=0 float=%f", tt.input, i, f, tt.wantFlt)
		}
	}
}

func TestParseFloatLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"42", 42, true},
		{"0.0", 0, true},
		{"0x1p2", 4, true},
		{"nope", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseFloatLiteral(tt.input)
		if ok != tt.ok {
			t.Fatalf("ParseFloatLiteral(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseFloatLiteral(%q)=%v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseIntegralLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"1", 1, true},
		{"1.0", 1, true},
		{"1e0", 1, true},
		{"0x1p0", 1, true},
		{"-1.0", -1, true},
		{"1.5", 0, false},
		{"0x1.8p1", 3, true},
		{"nope", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseIntegralLiteral(tt.input)
		if ok != tt.ok {
			t.Fatalf("ParseIntegralLiteral(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseIntegralLiteral(%q)=%v, want %v", tt.input, got, tt.want)
		}
	}
}

// Expectations come from the Lua 5.4 reference interpreter (tonumber).
func TestParse(t *testing.T) {
	inf := math.Inf(1)
	tests := []struct {
		input   string
		ok      bool
		isFloat bool
		i       int64
		f       float64
	}{
		{"10", true, false, 10, 0},
		{"+5", true, false, 5, 0},
		{"-0", true, false, 0, 0},
		{"  10  ", true, false, 10, 0},
		{"\t\v\f\r 5 \n", true, false, 5, 0},
		{"9223372036854775807", true, false, math.MaxInt64, 0},
		{"-9223372036854775808", true, false, math.MinInt64, 0},
		{"0x10", true, false, 16, 0},
		{"+0x5", true, false, 5, 0},
		{"-0x10", true, false, -16, 0},
		{"0XfF", true, false, 255, 0},
		{"0xffffffffffffffff", true, false, -1, 0},
		{"0xffffffffffffffffff", true, false, -1, 0},
		{"0x7fffffffffffffff", true, false, math.MaxInt64, 0},
		{"0x8000000000000000", true, false, math.MinInt64, 0},
		{"1e5", true, true, 0, 1e5},
		{"1E2", true, true, 0, 100},
		{"1e+2", true, true, 0, 100},
		{"25e-1", true, true, 0, 2.5},
		{".5", true, true, 0, 0.5},
		{"5.", true, true, 0, 5},
		{".5e1", true, true, 0, 5},
		{"-0.0", true, true, 0, math.Copysign(0, -1)},
		{"9223372036854775808", true, true, 0, 9223372036854775808},
		{"-9223372036854775809", true, true, 0, -9223372036854775809},
		{"1e400", true, true, 0, inf},
		{"-1e400", true, true, 0, -inf},
		{"1e-400", true, true, 0, 0},
		{"0x1p4", true, true, 0, 16},
		{"0x1P-1", true, true, 0, 0.5},
		{"0xA.8p0", true, true, 0, 10.5},
		{"0x.8", true, true, 0, 0.5},
		{"0x1.8", true, true, 0, 1.5},
		{"0x8.", true, true, 0, 8},
		{" 0x1.8 ", true, true, 0, 1.5},
		{"0x1P-1074", true, true, 0, 5e-324},
		{"", false, false, 0, 0},
		{"   ", false, false, 0, 0},
		{"-", false, false, 0, 0},
		{".", false, false, 0, 0},
		{"0x", false, false, 0, 0},
		{"0x.", false, false, 0, 0},
		{"0xp1", false, false, 0, 0},
		{"0x1p", false, false, 0, 0},
		{"1e", false, false, 0, 0},
		{"1e+", false, false, 0, 0},
		{"e5", false, false, 0, 0},
		{"1 2", false, false, 0, 0},
		{"1_0", false, false, 0, 0},
		{"0x1_0", false, false, 0, 0},
		{"0b101", false, false, 0, 0},
		{"0o17", false, false, 0, 0},
		{"1\x00", false, false, 0, 0},
		{"--1", false, false, 0, 0},
		{"- 1", false, false, 0, 0},
		{"1.5.2", false, false, 0, 0},
		{"0x1e", true, false, 30, 0},
		{"1p4", false, false, 0, 0},
		{"inf", false, false, 0, 0},
		{"-inf", false, false, 0, 0},
		{"Inf", false, false, 0, 0},
		{"infinity", false, false, 0, 0},
		{"nan", false, false, 0, 0},
		{"NaN", false, false, 0, 0},
		{"abc", false, false, 0, 0},
	}
	for _, tt := range tests {
		got, ok := Parse(tt.input)
		if ok != tt.ok {
			t.Fatalf("Parse(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if !ok {
			continue
		}
		if got.IsFloat != tt.isFloat {
			t.Fatalf("Parse(%q) IsFloat=%v, want %v", tt.input, got.IsFloat, tt.isFloat)
		}
		if tt.isFloat {
			if math.Float64bits(got.Float) != math.Float64bits(tt.f) {
				t.Fatalf("Parse(%q)=%v, want %v", tt.input, got.Float, tt.f)
			}
		} else if got.Int != tt.i {
			t.Fatalf("Parse(%q)=%d, want %d", tt.input, got.Int, tt.i)
		}
	}
}

// Expectations come from the Lua 5.4 reference interpreter (tonumber with base).
func TestParseBase(t *testing.T) {
	tests := []struct {
		input string
		base  int
		want  int64
		ok    bool
	}{
		{"ff", 16, 255, true},
		{"FF", 16, 255, true},
		{"z", 36, 35, true},
		{"Zz", 36, 35*36 + 35, true},
		{" 10 ", 2, 2, true},
		{"-ff", 16, -255, true},
		{"+11", 2, 3, true},
		{"777", 8, 511, true},
		{"10", 10, 10, true},
		{"7fffffffffffffff", 16, math.MaxInt64, true},
		{"ffffffffffffffff", 16, -1, true},
		{"10000000000000000", 16, 0, true},
		{"2", 2, 0, false},
		{"1e1", 10, 0, false},
		{"0x10", 16, 0, false},
		{"1.0", 10, 0, false},
		{"", 10, 0, false},
		{" ", 10, 0, false},
		{"-", 10, 0, false},
		{"1 1", 10, 0, false},
		{"1\x00", 10, 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseBase(tt.input, tt.base)
		if ok != tt.ok {
			t.Fatalf("ParseBase(%q, %d) ok=%v, want %v", tt.input, tt.base, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("ParseBase(%q, %d)=%d, want %d", tt.input, tt.base, got, tt.want)
		}
	}
}
