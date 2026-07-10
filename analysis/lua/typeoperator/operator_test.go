package typeoperator

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestBinaryOpPrimitiveArithmeticAndConcat(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		op    string
		right typ.Type
		want  typ.Type
	}{
		{name: "integer addition", left: typ.Integer, op: "+", right: typ.LiteralInt(2), want: typ.Integer},
		{name: "mixed numeric multiplication", left: typ.Integer, op: "*", right: typ.Number, want: typ.Number},
		{name: "division is number", left: typ.Integer, op: "/", right: typ.Integer, want: typ.Number},
		{name: "integer modulo", left: typ.Integer, op: "%", right: typ.Integer, want: typ.Integer},
		{name: "numeric modulo", left: typ.Number, op: "%", right: typ.Integer, want: typ.Number},
		{name: "integer floor division", left: typ.Integer, op: "//", right: typ.Integer, want: typ.Integer},
		{name: "numeric floor division", left: typ.Number, op: "//", right: typ.Integer, want: typ.Number},
		{name: "bitwise and", left: typ.Integer, op: "&", right: typ.LiteralInt(3), want: typ.Integer},
		{name: "concat string and integer", left: typ.String, op: "..", right: typ.Integer, want: typ.String},
		{name: "concat number and string literal", left: typ.Number, op: "..", right: typ.LiteralString("x"), want: typ.String},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BinaryOp(tt.left, tt.op, tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, %q, %v) failed", tt.left, tt.op, tt.right)
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestBinaryOpPrimitiveComparisons(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		op    string
		right typ.Type
	}{
		{name: "equality across unrelated types", left: typ.String, op: "==", right: typ.Number},
		{name: "inequality with nil", left: typeexpr.Optional(typ.String), op: "~=", right: typ.Nil},
		{name: "numeric less than", left: typ.Integer, op: "<", right: typ.Number},
		{name: "string literal greater equal", left: typ.LiteralString("b"), op: ">=", right: typ.String},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BinaryOp(tt.left, tt.op, tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, %q, %v) failed", tt.left, tt.op, tt.right)
			}
			assertType(t, got, typ.Boolean)
		})
	}

	if _, ok := BinaryOp(typ.String, "<", typ.Number); ok {
		t.Fatal("BinaryOp(string, <, number) succeeded")
	}
}

func TestUnaryOpPrimitives(t *testing.T) {
	tests := []struct {
		name    string
		op      string
		operand typ.Type
		want    typ.Type
	}{
		{name: "not optional", op: "not", operand: typeexpr.Optional(typ.String), want: typ.Boolean},
		{name: "integer negation", op: "-", operand: typ.Integer, want: typ.Integer},
		{name: "number negation", op: "-", operand: typ.Number, want: typ.Number},
		{name: "bitwise not", op: "~", operand: typ.LiteralInt(7), want: typ.Integer},
		{name: "string length", op: "#", operand: typ.String, want: typ.Integer},
		{name: "array length", op: "#", operand: typ.NewArray(typ.String), want: typ.Integer},
		{name: "record length", op: "#", operand: typetable.NewRecord().Field("x", typ.Number).Build(), want: typ.Integer},
		{name: "builtin table marker length", op: "#", operand: typetable.BuiltinTopMarker(), want: typ.Integer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := UnaryOp(tt.op, tt.operand)
			if !ok {
				t.Fatalf("UnaryOp(%q, %v) failed", tt.op, tt.operand)
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestOperatorOptionalAndNilRejection(t *testing.T) {
	if _, ok := BinaryOp(typeexpr.Optional(typ.Integer), "+", typ.Integer); ok {
		t.Fatal("BinaryOp(optional integer, +, integer) succeeded")
	}
	if _, ok := BinaryOp(typ.Number, "+", typeexpr.Optional(typ.Number)); ok {
		t.Fatal("BinaryOp(number, +, optional number) succeeded")
	}
	if _, ok := BinaryOp(typ.Nil, "..", typ.String); ok {
		t.Fatal("BinaryOp(nil, .., string) succeeded")
	}
	if _, ok := UnaryOp("#", typeexpr.Optional(typ.NewArray(typ.String))); ok {
		t.Fatal("UnaryOp(#, optional array) succeeded")
	}
	if _, ok := UnaryOp("-", typ.Nil); ok {
		t.Fatal("UnaryOp(-, nil) succeeded")
	}

	got, ok := BinaryOp(typ.Nil, "==", typeexpr.Optional(typ.Integer))
	if !ok {
		t.Fatal("BinaryOp(nil, ==, optional integer) failed")
	}
	assertType(t, got, typ.Boolean)

	got, ok = UnaryOp("not", typ.Nil)
	if !ok {
		t.Fatal("UnaryOp(not, nil) failed")
	}
	assertType(t, got, typ.Boolean)
}

func TestConcatOptionalOperandProjectsPresentResult(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		right typ.Type
	}{
		{name: "optional string right", left: typ.String, right: typeexpr.Optional(typ.String)},
		{name: "optional string left", left: typeexpr.Optional(typ.String), right: typ.String},
		{name: "optional number right", left: typ.String, right: typeexpr.Optional(typ.Number)},
		{name: "nil union string right", left: typ.String, right: typeexpr.Union(typ.Nil, typ.String)},
		{name: "nil union number right", left: typ.String, right: typeexpr.Union(typ.Nil, typ.Number)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BinaryOp(tt.left, "..", tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, .., %v) failed", tt.left, tt.right)
			}
			assertType(t, got, typ.String)
		})
	}
}

func TestConcatOptionalNonConcatenableOperandStillRejected(t *testing.T) {
	if _, ok := BinaryOp(typ.String, "..", typeexpr.Optional(typ.NewArray(typ.String))); ok {
		t.Fatal("BinaryOp(string, .., optional array) succeeded")
	}
}

func TestOperatorUnionDistributionRequiresEveryVariant(t *testing.T) {
	got, ok := BinaryOp(typeexpr.Union(typ.Integer, typ.Number), "+", typ.Integer)
	if !ok {
		t.Fatal("BinaryOp(integer | number, +, integer) failed")
	}
	assertType(t, got, typ.Number)

	if _, ok := BinaryOp(typeexpr.Union(typ.Integer, typ.String), "+", typ.Integer); ok {
		t.Fatal("BinaryOp(integer | string, +, integer) succeeded")
	}

	got, ok = UnaryOp("#", typeexpr.Union(typ.String, typ.NewArray(typ.Number)))
	if !ok {
		t.Fatal("UnaryOp(#, string | number[]) failed")
	}
	assertType(t, got, typ.Integer)

	if _, ok := UnaryOp("#", typeexpr.Union(typ.String, typ.Number)); ok {
		t.Fatal("UnaryOp(#, string | number) succeeded")
	}
}

func TestBinaryOpLogicalTruthiness(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		op    string
		right typ.Type
		want  typ.Type
	}{
		{name: "nil and returns nil", left: typ.Nil, op: "and", right: typ.String, want: typ.Nil},
		{name: "false or returns right", left: typ.False, op: "or", right: typ.String, want: typ.String},
		{name: "true and returns right", left: typ.True, op: "and", right: typ.Number, want: typ.Number},
		{name: "truthy string or returns left", left: typ.String, op: "or", right: typ.Number, want: typ.String},
		{name: "boolean and splits false or right", left: typ.Boolean, op: "and", right: typ.String, want: typeexpr.Union(typ.False, typ.String)},
		{name: "boolean or splits true or right", left: typ.Boolean, op: "or", right: typ.String, want: typeexpr.Union(typ.True, typ.String)},
		{name: "any and returns falsy condition or right", left: typ.Any, op: "and", right: typ.String, want: typeexpr.Union(typ.Nil, typ.False, typ.String)},
		{name: "unknown and returns falsy condition or right literal", left: typ.Unknown, op: "and", right: typ.LiteralInt(1), want: typeexpr.Union(typ.Nil, typ.False, typ.LiteralInt(1))},
		{name: "unknown or is unknown", left: typ.Unknown, op: "or", right: typ.String, want: typ.Unknown},
		{name: "truthy left or unknown stays left", left: typ.True, op: "or", right: typ.Unknown, want: typ.True},
		{name: "falsey left and unknown stays left", left: typ.Nil, op: "and", right: typ.Unknown, want: typ.Nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BinaryOp(tt.left, tt.op, tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, %q, %v) failed", tt.left, tt.op, tt.right)
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestBinaryOpLogicalUnionDistribution(t *testing.T) {
	got, ok := BinaryOp(typeexpr.Union(typ.Nil, typ.String), "or", typ.Number)
	if !ok {
		t.Fatal("BinaryOp(nil | string, or, number) failed")
	}
	assertType(t, got, typeexpr.Union(typ.String, typ.Number))

	got, ok = BinaryOp(typeexpr.Union(typ.False, typ.String), "and", typ.Number)
	if !ok {
		t.Fatal("BinaryOp(false | string, and, number) failed")
	}
	assertType(t, got, typeexpr.Union(typ.False, typ.Number))

	got, ok = BinaryOp(typeexpr.Union(typ.Unknown, typ.Nil), "or", typ.Number)
	if !ok {
		t.Fatal("BinaryOp(unknown | nil, or, number) failed")
	}
	assertType(t, got, typ.Unknown)

	got, ok = BinaryOp(BinaryOpMust(t, typ.Unknown, "and", typ.LiteralInt(1)), "or", typ.LiteralInt(0))
	if !ok {
		t.Fatal("BinaryOp((unknown and 1), or, 0) failed")
	}
	assertType(t, got, typeexpr.Union(typ.LiteralInt(1), typ.LiteralInt(0)))
}

func BinaryOpMust(t *testing.T, left typ.Type, op string, right typ.Type) typ.Type {
	t.Helper()
	got, ok := BinaryOp(left, op, right)
	if !ok {
		t.Fatalf("BinaryOp(%v, %q, %v) failed", left, op, right)
	}
	return got
}

func TestOperatorAnyUnknownNeverPolicy(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		op    string
		right typ.Type
		want  typ.Type
	}{
		{name: "any arithmetic", left: typ.Any, op: "+", right: typ.String, want: typ.Unknown},
		{name: "any concat", left: typ.Any, op: "..", right: typ.String, want: typ.Unknown},
		{name: "any ordering", left: typ.Any, op: "<", right: typ.Number, want: typ.Boolean},
		{name: "any bitwise", left: typ.Any, op: "&", right: typ.Integer, want: typ.Unknown},
		{name: "unknown arithmetic", left: typ.Unknown, op: "+", right: typ.String, want: typ.Unknown},
		{name: "never arithmetic", left: typ.Never, op: "+", right: typ.Integer, want: typ.Never},
		{name: "unknown relation", left: typ.Unknown, op: "<", right: typ.Number, want: typ.Boolean},
		{name: "never equality is invariant boolean", left: typ.Never, op: "==", right: typ.Integer, want: typ.Boolean},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BinaryOp(tt.left, tt.op, tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, %q, %v) failed", tt.left, tt.op, tt.right)
			}
			assertType(t, got, tt.want)
		})
	}

	got, ok := UnaryOp("-", typ.Unknown)
	if !ok {
		t.Fatal("UnaryOp(-, unknown) failed")
	}
	assertType(t, got, typ.Unknown)

	got, ok = UnaryOp("-", typ.Any)
	if !ok {
		t.Fatal("UnaryOp(-, any) failed")
	}
	assertType(t, got, typ.Unknown)

	got, ok = UnaryOp("#", typ.Any)
	if !ok {
		t.Fatal("UnaryOp(#, any) failed")
	}
	assertType(t, got, typ.Integer)

	got, ok = UnaryOp("#", typ.Unknown)
	if !ok {
		t.Fatal("UnaryOp(#, unknown) failed")
	}
	assertType(t, got, typ.Integer)

	got, ok = UnaryOp("-", typ.Never)
	if !ok {
		t.Fatal("UnaryOp(-, never) failed")
	}
	assertType(t, got, typ.Never)

	got, ok = UnaryOp("not", typ.Unknown)
	if !ok {
		t.Fatal("UnaryOp(not, unknown) failed")
	}
	assertType(t, got, typ.Boolean)
}

func TestOperatorMetamethodPrecedence(t *testing.T) {
	lenRecord := recordWithMetamethod("__len", typ.Func().Returns(typ.String).Build())
	got, ok := UnaryOp("#", lenRecord)
	if !ok {
		t.Fatal("UnaryOp(#, record with __len) failed")
	}
	assertType(t, got, typ.String)

	left := recordWithMetamethod("__add", typ.Func().Returns(typ.String).Build())
	right := recordWithMetamethod("__add", typ.Func().Returns(typ.Number).Build())
	got, ok = BinaryOp(left, "+", right)
	if !ok {
		t.Fatal("BinaryOp(record __add, +, record __add) failed")
	}
	assertType(t, got, typ.String)

	noAdd := typetable.NewRecord().Build()
	got, ok = BinaryOp(noAdd, "+", right)
	if !ok {
		t.Fatal("BinaryOp(record, +, right __add) failed")
	}
	assertType(t, got, typ.Number)
}

func TestOperatorSwappedComparisonMetamethod(t *testing.T) {
	left := typetable.NewRecord().Build()
	right := recordWithMetamethod("__lt", typ.Func().Returns(typ.Boolean).Build())

	got, ok := BinaryOp(left, ">", right)
	if !ok {
		t.Fatal("BinaryOp(left, >, right __lt) failed")
	}
	assertType(t, got, typ.Boolean)
}

func assertType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}

func recordWithMetamethod(name string, mt typ.Type) *typ.Record {
	return typetable.NewRecord().
		Metatable(typetable.NewRecord().Field(name, mt).Build()).
		Build()
}
