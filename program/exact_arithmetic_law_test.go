package program_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestExactArithmeticLiteralOwnsClosedLuaNumericSemantics(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	float := func(value float64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(value)}
	}
	cases := []struct {
		left, right keyspace.LiteralValue
		op          flowkind.BinaryOp
		want        keyspace.LiteralValue
		ok          bool
	}{
		{integer(10), integer(5), flowkind.BinaryAdd, integer(15), true},
		{integer(math.MaxInt64), integer(1), flowkind.BinaryAdd, integer(math.MinInt64), true},
		{integer(7), integer(9), flowkind.BinarySub, integer(-2), true},
		{integer(7), integer(-3), flowkind.BinaryMul, integer(-21), true},
		{integer(7), integer(2), flowkind.BinaryDiv, float(3.5), true},
		{integer(-7), integer(3), flowkind.BinaryIDiv, integer(-3), true},
		{integer(-7), integer(3), flowkind.BinaryMod, integer(2), true},
		{float(7.5), integer(2), flowkind.BinaryIDiv, float(3), true},
		{integer(2), integer(3), flowkind.BinaryPow, float(8), true},
		{integer(1), integer(0), flowkind.BinaryIDiv, keyspace.LiteralValue{}, false},
		{integer(1), integer(0), flowkind.BinaryMod, keyspace.LiteralValue{}, false},
	}
	for _, test := range cases {
		got, ok := program.ExactArithmeticLiteral(test.left, test.right, test.op)
		if ok != test.ok || ok && got != test.want {
			t.Fatalf("ExactArithmeticLiteral(%+v,%+v,%d) = %+v/%v, want %+v/%v", test.left, test.right, test.op, got, ok, test.want, test.ok)
		}
	}
	if _, ok := program.ExactArithmeticLiteral(integer(1), integer(2), flowkind.BinaryEqual); ok {
		t.Fatal("non-arithmetic operator accepted")
	}
}

func TestExactUnaryNegLiteralOwnsClosedLuaNumericSemantics(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	float := func(value float64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(value)}
	}
	for _, test := range []struct {
		input keyspace.LiteralValue
		want  keyspace.LiteralValue
		ok    bool
	}{
		{integer(1), integer(-1), true},
		{integer(math.MinInt64), integer(math.MinInt64), true},
		{float(1.5), float(-1.5), true},
		{keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true}, keyspace.LiteralValue{}, false},
	} {
		got, ok := program.ExactUnaryNegLiteral(test.input)
		if ok != test.ok || ok && got != test.want {
			t.Fatalf("ExactUnaryNegLiteral(%+v) = %+v/%v, want %+v/%v", test.input, got, ok, test.want, test.ok)
		}
	}
}
