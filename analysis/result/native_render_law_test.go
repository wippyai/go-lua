package result

import (
	"math"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestNativeRenderCatalogsAreTotalOverTheirSealedVocabulariesLaw(t *testing.T) {
	representations := map[cold.NumericRepresentation]string{
		cold.NumericRepresentationInteger: "integer",
		cold.NumericRepresentationFloat:   "float",
		cold.NumericRepresentationNumber:  "number",
	}
	for representation, spelling := range representations {
		rendered, ok := nativeNumericRepresentation(representation)
		if !ok || rendered != spelling {
			t.Fatalf("representation %d renders as %q, want %q", representation, rendered, spelling)
		}
	}
	if _, ok := nativeNumericRepresentation(cold.NumericRepresentationInvalid); ok {
		t.Fatal("the invalid representation rendered")
	}
	if _, ok := nativeNumericRepresentation(cold.NumericRepresentationNumber + 1); ok {
		t.Fatal("a representation above the sealed vocabulary rendered")
	}

	operators := map[flowkind.BinaryOp]string{
		flowkind.BinaryAdd:  "add",
		flowkind.BinarySub:  "sub",
		flowkind.BinaryMul:  "mul",
		flowkind.BinaryDiv:  "div",
		flowkind.BinaryIDiv: "idiv",
		flowkind.BinaryMod:  "mod",
		flowkind.BinaryPow:  "pow",
	}
	for op := flowkind.BinaryAdd; op <= flowkind.BinaryPow; op++ {
		rendered, ok := nativeArithmeticOperator(op)
		if !ok || rendered != operators[op] {
			t.Fatalf("arithmetic operator %d renders as %q, want %q", op, rendered, operators[op])
		}
	}
	for op := flowkind.BinaryConcat; op <= flowkind.BinaryGreaterEqual; op++ {
		if _, ok := nativeArithmeticOperator(op); ok {
			t.Fatalf("non-arithmetic operator %d rendered as an arithmetic operator", op)
		}
	}

	divisors := map[cold.ArithmeticDivisorProperty]string{
		cold.ArithmeticDivisorNone:               "",
		cold.ArithmeticDivisorNonzero:            "nonzero",
		cold.ArithmeticDivisorNonzeroNotMinusOne: "nonzero_not_minus_one",
	}
	for property, spelling := range divisors {
		rendered, ok := nativeArithmeticDivisor(property)
		if !ok || rendered != spelling {
			t.Fatalf("divisor property %d renders as %q, want %q", property, rendered, spelling)
		}
	}
	if _, ok := nativeArithmeticDivisor(cold.ArithmeticDivisorNonzeroNotMinusOne + 1); ok {
		t.Fatal("a divisor property above the sealed vocabulary rendered")
	}
}

func TestNativeLiteralRenderingIsOneTableLaw(t *testing.T) {
	for _, rendering := range []struct {
		literal        keyspace.LiteralValue
		representation string
		value          string
	}{
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool}, representation: "boolean", value: "false"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true}, representation: "boolean", value: "true"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: -7}, representation: "integer", value: "-7"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(42)}, representation: "float", value: "42.0"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(0.5)}, representation: "float", value: "0.5"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1e21)}, representation: "float", value: "1e+21"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: ""}, representation: "string", value: `""`},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "a\"b"}, representation: "string", value: `"a\"b"`},
	} {
		representation, value, ok := renderNativeLiteral(rendering.literal)
		if !ok || representation != rendering.representation || value != rendering.value {
			t.Fatalf("literal kind %d renders as (%q,%q), want (%q,%q)", rendering.literal.Kind, representation, value, rendering.representation, rendering.value)
		}
	}
	for _, unrenderable := range []keyspace.LiteralValue{
		{},
		{Kind: keyspace.LiteralString + 1},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.NaN())},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Inf(1))},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Inf(-1))},
	} {
		if _, _, ok := renderNativeLiteral(unrenderable); ok {
			t.Fatalf("literal kind %d rendered a value with no exact spelling", unrenderable.Kind)
		}
	}
	if representation, value, ok := renderNativeNil(); !ok || representation != "nil" || value != "nil" {
		t.Fatalf("nil renders as (%q,%q), want (nil,nil)", representation, value)
	}
	if _, _, ok := renderNativeExactScalar(valuedomain.ExactScalar{}); ok {
		t.Fatal("an unclassified exact scalar rendered")
	}
	if _, _, ok := renderNativeScalarSummary(compiledNativeScalarSource{}); ok {
		t.Fatal("an invalid scalar summary rendered")
	}
}
