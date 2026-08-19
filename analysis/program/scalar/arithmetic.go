package scalar

import (
	"math"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// ExactArithmeticLiteral is Program's sole concrete arithmetic evaluator for
// two exact numeric literals. It is shared by reusable artifact summaries and
// abstract domains, preventing a Link- or Result-side fold from becoming a
// second arithmetic authority.
func ExactArithmeticLiteral(left, right keyspace.LiteralValue, op flowkind.BinaryOp) (keyspace.LiteralValue, bool) {
	if !flowkind.IsBinaryArithmetic(op) || !numericLiteral(left) || !numericLiteral(right) {
		return keyspace.LiteralValue{}, false
	}
	if left.Kind == keyspace.LiteralInteger && right.Kind == keyspace.LiteralInteger {
		a, b := left.Integer, right.Integer
		switch op {
		case flowkind.BinaryAdd:
			return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: int64(uint64(a) + uint64(b))}, true
		case flowkind.BinarySub:
			return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: int64(uint64(a) - uint64(b))}, true
		case flowkind.BinaryMul:
			return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: int64(uint64(a) * uint64(b))}, true
		case flowkind.BinaryIDiv:
			value, ok := luaIntegerFloorDiv(a, b)
			return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, ok
		case flowkind.BinaryMod:
			value, ok := luaIntegerMod(a, b)
			return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, ok
		}
	}
	a, b := literalFloat(left), literalFloat(right)
	var result float64
	switch op {
	case flowkind.BinaryAdd:
		result = a + b
	case flowkind.BinarySub:
		result = a - b
	case flowkind.BinaryMul:
		result = a * b
	case flowkind.BinaryDiv:
		result = a / b
	case flowkind.BinaryIDiv:
		if b == 0 || math.IsNaN(a) || math.IsNaN(b) {
			return keyspace.LiteralValue{}, false
		}
		result = math.Floor(a / b)
	case flowkind.BinaryMod:
		if b == 0 || math.IsNaN(a) || math.IsNaN(b) {
			return keyspace.LiteralValue{}, false
		}
		result = a - math.Floor(a/b)*b
	case flowkind.BinaryPow:
		result = math.Pow(a, b)
	default:
		return keyspace.LiteralValue{}, false
	}
	return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(result)}, true
}

// ExactUnaryNegLiteral is Program's sole concrete evaluator for an authored
// unary numeric negation. Keeping this beside ExactArithmeticLiteral prevents
// transformer summaries and analysis domains from growing a second literal
// arithmetic authority merely to recognize guards such as x ~= -1.
func ExactUnaryNegLiteral(value keyspace.LiteralValue) (keyspace.LiteralValue, bool) {
	switch value.Kind {
	case keyspace.LiteralInteger:
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: int64(uint64(0) - uint64(value.Integer))}, true
	case keyspace.LiteralFloat:
		return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(-math.Float64frombits(value.FloatBits))}, true
	default:
		return keyspace.LiteralValue{}, false
	}
}

func numericLiteral(value keyspace.LiteralValue) bool {
	return value.Kind == keyspace.LiteralInteger || value.Kind == keyspace.LiteralFloat
}

func literalFloat(value keyspace.LiteralValue) float64 {
	if value.Kind == keyspace.LiteralInteger {
		return float64(value.Integer)
	}
	return math.Float64frombits(value.FloatBits)
}

func luaIntegerFloorDiv(left, right int64) (int64, bool) {
	if right == 0 {
		return 0, false
	}
	if right == -1 {
		return int64(uint64(0) - uint64(left)), true
	}
	quotient, remainder := left/right, left%right
	if remainder != 0 && (remainder < 0) != (right < 0) {
		quotient--
	}
	return quotient, true
}

func luaIntegerMod(left, right int64) (int64, bool) {
	if right == 0 {
		return 0, false
	}
	if right == -1 {
		return 0, true
	}
	remainder := left % right
	if remainder != 0 && (remainder < 0) != (right < 0) {
		remainder += right
	}
	return remainder, true
}
