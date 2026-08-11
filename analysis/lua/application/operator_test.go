package application

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/program/flow/kind"
)

func TestBinaryOpPrimitiveArithmeticAndConcat(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		op    kind.BinaryOp
		right typ.Type
		want  typ.Type
	}{
		{name: "integer addition", left: typ.Integer, op: kind.BinaryAdd, right: typ.LiteralInt(2), want: typ.Integer},
		{name: "mixed numeric multiplication", left: typ.Integer, op: kind.BinaryMul, right: typ.Number, want: typ.Number},
		{name: "division is number", left: typ.Integer, op: kind.BinaryDiv, right: typ.Integer, want: typ.Number},
		{name: "integer modulo", left: typ.Integer, op: kind.BinaryMod, right: typ.Integer, want: typ.Integer},
		{name: "numeric modulo", left: typ.Number, op: kind.BinaryMod, right: typ.Integer, want: typ.Number},
		{name: "integer floor division", left: typ.Integer, op: kind.BinaryIDiv, right: typ.Integer, want: typ.Integer},
		{name: "numeric floor division", left: typ.Number, op: kind.BinaryIDiv, right: typ.Integer, want: typ.Number},
		{name: "bitwise and", left: typ.Integer, op: kind.BinaryBitAnd, right: typ.LiteralInt(3), want: typ.Integer},
		{name: "concat string and integer", left: typ.String, op: kind.BinaryConcat, right: typ.Integer, want: typ.String},
		{name: "concat number and string literal", left: typ.Number, op: kind.BinaryConcat, right: typ.LiteralString("x"), want: typ.String},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BinaryResult(tt.left, tt.op, tt.right)
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
		op    kind.BinaryOp
		right typ.Type
	}{
		{name: "equality across unrelated types", left: typ.String, op: kind.BinaryEqual, right: typ.Number},
		{name: "inequality with nil", left: typeexpr.Optional(typ.String), op: kind.BinaryNotEqual, right: typ.Nil},
		{name: "numeric less than", left: typ.Integer, op: kind.BinaryLess, right: typ.Number},
		{name: "string literal greater equal", left: typ.LiteralString("b"), op: kind.BinaryGreaterEqual, right: typ.String},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BinaryResult(tt.left, tt.op, tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, %q, %v) failed", tt.left, tt.op, tt.right)
			}
			assertType(t, got, typ.Boolean)
		})
	}

	if _, ok := BinaryResult(typ.String, kind.BinaryLess, typ.Number); ok {
		t.Fatal("BinaryOp(string, <, number) succeeded")
	}
}

func TestUnaryOpPrimitives(t *testing.T) {
	tests := []struct {
		name    string
		op      kind.UnaryOp
		operand typ.Type
		want    typ.Type
	}{
		{name: "not optional", op: kind.UnaryNot, operand: typeexpr.Optional(typ.String), want: typ.Boolean},
		{name: "integer negation", op: kind.UnaryNeg, operand: typ.Integer, want: typ.Integer},
		{name: "number negation", op: kind.UnaryNeg, operand: typ.Number, want: typ.Number},
		{name: "bitwise not", op: kind.UnaryBitNot, operand: typ.LiteralInt(7), want: typ.Integer},
		{name: "string length", op: kind.UnaryLen, operand: typ.String, want: typ.Integer},
		{name: "array length", op: kind.UnaryLen, operand: typ.NewArray(typ.String), want: typ.Integer},
		{name: "record length", op: kind.UnaryLen, operand: typetable.NewRecord().Field("x", typ.Number).Build(), want: typ.Integer},
		{name: "builtin table marker length", op: kind.UnaryLen, operand: typ.BuiltinTableTopMarker(), want: typ.Integer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := UnaryResult(tt.op, tt.operand)
			if !ok {
				t.Fatalf("UnaryOp(%q, %v) failed", tt.op, tt.operand)
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestOperatorOptionalAndNilRejection(t *testing.T) {
	if _, ok := BinaryResult(typeexpr.Optional(typ.Integer), kind.BinaryAdd, typ.Integer); ok {
		t.Fatal("BinaryOp(optional integer, +, integer) succeeded")
	}
	if _, ok := BinaryResult(typ.Number, kind.BinaryAdd, typeexpr.Optional(typ.Number)); ok {
		t.Fatal("BinaryOp(number, +, optional number) succeeded")
	}
	if _, ok := BinaryResult(typ.Nil, kind.BinaryConcat, typ.String); ok {
		t.Fatal("BinaryOp(nil, .., string) succeeded")
	}
	if _, ok := UnaryResult(kind.UnaryLen, typeexpr.Optional(typ.NewArray(typ.String))); ok {
		t.Fatal("UnaryOp(#, optional array) succeeded")
	}
	if _, ok := UnaryResult(kind.UnaryNeg, typ.Nil); ok {
		t.Fatal("UnaryOp(-, nil) succeeded")
	}

	got, ok := BinaryResult(typ.Nil, kind.BinaryEqual, typeexpr.Optional(typ.Integer))
	if !ok {
		t.Fatal("BinaryOp(nil, ==, optional integer) failed")
	}
	assertType(t, got, typ.Boolean)

	got, ok = UnaryResult(kind.UnaryNot, typ.Nil)
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
			got, ok := BinaryResult(tt.left, kind.BinaryConcat, tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, .., %v) failed", tt.left, tt.right)
			}
			assertType(t, got, typ.String)
		})
	}
}

func TestConcatOptionalNonConcatenableOperandStillRejected(t *testing.T) {
	if _, ok := BinaryResult(typ.String, kind.BinaryConcat, typeexpr.Optional(typ.NewArray(typ.String))); ok {
		t.Fatal("BinaryOp(string, .., optional array) succeeded")
	}
}

func TestOperatorUnionDistributionRequiresEveryVariant(t *testing.T) {
	got, ok := BinaryResult(typeexpr.Union(typ.Integer, typ.Number), kind.BinaryAdd, typ.Integer)
	if !ok {
		t.Fatal("BinaryOp(integer | number, +, integer) failed")
	}
	assertType(t, got, typ.Number)

	if _, ok := BinaryResult(typeexpr.Union(typ.Integer, typ.String), kind.BinaryAdd, typ.Integer); ok {
		t.Fatal("BinaryOp(integer | string, +, integer) succeeded")
	}

	got, ok = UnaryResult(kind.UnaryLen, typeexpr.Union(typ.String, typ.NewArray(typ.Number)))
	if !ok {
		t.Fatal("UnaryOp(#, string | number[]) failed")
	}
	assertType(t, got, typ.Integer)

	if _, ok := UnaryResult(kind.UnaryLen, typeexpr.Union(typ.String, typ.Number)); ok {
		t.Fatal("UnaryOp(#, string | number) succeeded")
	}
}

func TestOperatorTraversalHasNoSemanticDepthLimit(t *testing.T) {
	deep := typ.Type(typ.Integer)
	const aliases = 300
	for i := 0; i < aliases; i++ {
		deep = &typ.Alias{Name: "Deep", Target: deep}
	}
	got, ok := BinaryResult(deep, kind.BinaryAdd, typ.Integer)
	if !ok {
		t.Fatalf("BinaryOp rejected a finite %d-alias chain", aliases)
	}
	assertType(t, got, typ.Integer)
	got, ok = UnaryResult(kind.UnaryNeg, deep)
	if !ok {
		t.Fatalf("UnaryOp rejected a finite %d-alias chain", aliases)
	}
	assertType(t, got, typ.Integer)
}

func TestOperatorUnionUsesExactRegularGraphBasis(t *testing.T) {
	loop := &typ.Alias{Name: "Loop"}
	union := &typ.Union{Members: []typ.Type{typ.Integer, loop}}
	loop.Target = union

	got, ok := BinaryResult(union, kind.BinaryAdd, typ.Integer)
	if !ok {
		t.Fatal("BinaryOp rejected a productive recursive numeric union")
	}
	assertType(t, got, typ.Integer)

	badLoop := &typ.Alias{Name: "BadLoop"}
	badUnion := &typ.Union{Members: []typ.Type{typ.Integer, typ.String, badLoop}}
	badLoop.Target = badUnion
	if _, ok := BinaryResult(badUnion, kind.BinaryAdd, typ.Integer); ok {
		t.Fatal("BinaryOp accepted a recursive union with a non-numeric leaf")
	}

	unproductive := &typ.Alias{Name: "Unproductive"}
	unproductive.Target = unproductive
	if _, ok := UnaryResult(kind.UnaryNeg, unproductive); ok {
		t.Fatal("UnaryOp accepted an unproductive alias cycle")
	}
}

func TestSelectResultLogicalTruthiness(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		op    kind.SelectOp
		right typ.Type
		want  typ.Type
	}{
		{name: "nil and returns nil", left: typ.Nil, op: kind.SelectAnd, right: typ.String, want: typ.Nil},
		{name: "false or returns right", left: typ.False, op: kind.SelectOr, right: typ.String, want: typ.String},
		{name: "true and returns right", left: typ.True, op: kind.SelectAnd, right: typ.Number, want: typ.Number},
		{name: "truthy string or returns left", left: typ.String, op: kind.SelectOr, right: typ.Number, want: typ.String},
		{name: "boolean and splits false or right", left: typ.Boolean, op: kind.SelectAnd, right: typ.String, want: typeexpr.Union(typ.False, typ.String)},
		{name: "boolean or splits true or right", left: typ.Boolean, op: kind.SelectOr, right: typ.String, want: typeexpr.Union(typ.True, typ.String)},
		{name: "any and returns falsy condition or right", left: typ.Any, op: kind.SelectAnd, right: typ.String, want: typeexpr.Union(typ.Nil, typ.False, typ.String)},
		{name: "unknown and returns falsy condition or right literal", left: typ.Unknown, op: kind.SelectAnd, right: typ.LiteralInt(1), want: typeexpr.Union(typ.Nil, typ.False, typ.LiteralInt(1))},
		{name: "unknown or is unknown", left: typ.Unknown, op: kind.SelectOr, right: typ.String, want: typ.Unknown},
		{name: "truthy left or unknown stays left", left: typ.True, op: kind.SelectOr, right: typ.Unknown, want: typ.True},
		{name: "falsey left and unknown stays left", left: typ.Nil, op: kind.SelectAnd, right: typ.Unknown, want: typ.Nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SelectResult(tt.left, tt.op, tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, %q, %v) failed", tt.left, tt.op, tt.right)
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestSelectResultLogicalUnionDistribution(t *testing.T) {
	got, ok := SelectResult(typeexpr.Union(typ.Nil, typ.String), kind.SelectOr, typ.Number)
	if !ok {
		t.Fatal("BinaryOp(nil | string, or, number) failed")
	}
	assertType(t, got, typeexpr.Union(typ.String, typ.Number))

	got, ok = SelectResult(typeexpr.Union(typ.False, typ.String), kind.SelectAnd, typ.Number)
	if !ok {
		t.Fatal("BinaryOp(false | string, and, number) failed")
	}
	assertType(t, got, typeexpr.Union(typ.False, typ.Number))

	got, ok = SelectResult(typeexpr.Union(typ.Unknown, typ.Nil), kind.SelectOr, typ.Number)
	if !ok {
		t.Fatal("BinaryOp(unknown | nil, or, number) failed")
	}
	assertType(t, got, typ.Unknown)

	got, ok = SelectResult(SelectResultMust(t, typ.Unknown, kind.SelectAnd, typ.LiteralInt(1)), kind.SelectOr, typ.LiteralInt(0))
	if !ok {
		t.Fatal("BinaryOp((unknown and 1), or, 0) failed")
	}
	assertType(t, got, typeexpr.Union(typ.LiteralInt(1), typ.LiteralInt(0)))
}

func SelectResultMust(t *testing.T, left typ.Type, op kind.SelectOp, right typ.Type) typ.Type {
	t.Helper()
	got, ok := SelectResult(left, op, right)
	if !ok {
		t.Fatalf("BinaryOp(%v, %q, %v) failed", left, op, right)
	}
	return got
}

func TestOperatorAnyUnknownNeverPolicy(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		op    kind.BinaryOp
		right typ.Type
		want  typ.Type
	}{
		{name: "any arithmetic", left: typ.Any, op: kind.BinaryAdd, right: typ.String, want: typ.Unknown},
		{name: "any concat", left: typ.Any, op: kind.BinaryConcat, right: typ.String, want: typ.Unknown},
		{name: "any ordering", left: typ.Any, op: kind.BinaryLess, right: typ.Number, want: typ.Boolean},
		{name: "any bitwise", left: typ.Any, op: kind.BinaryBitAnd, right: typ.Integer, want: typ.Unknown},
		{name: "unknown arithmetic", left: typ.Unknown, op: kind.BinaryAdd, right: typ.String, want: typ.Unknown},
		{name: "never arithmetic", left: typ.Never, op: kind.BinaryAdd, right: typ.Integer, want: typ.Never},
		{name: "unknown relation", left: typ.Unknown, op: kind.BinaryLess, right: typ.Number, want: typ.Boolean},
		{name: "never equality is invariant boolean", left: typ.Never, op: kind.BinaryEqual, right: typ.Integer, want: typ.Boolean},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BinaryResult(tt.left, tt.op, tt.right)
			if !ok {
				t.Fatalf("BinaryOp(%v, %q, %v) failed", tt.left, tt.op, tt.right)
			}
			assertType(t, got, tt.want)
		})
	}

	got, ok := UnaryResult(kind.UnaryNeg, typ.Unknown)
	if !ok {
		t.Fatal("UnaryOp(-, unknown) failed")
	}
	assertType(t, got, typ.Unknown)

	got, ok = UnaryResult(kind.UnaryNeg, typ.Any)
	if !ok {
		t.Fatal("UnaryOp(-, any) failed")
	}
	assertType(t, got, typ.Unknown)

	got, ok = UnaryResult(kind.UnaryLen, typ.Any)
	if !ok {
		t.Fatal("UnaryOp(#, any) failed")
	}
	assertType(t, got, typ.Integer)

	got, ok = UnaryResult(kind.UnaryLen, typ.Unknown)
	if !ok {
		t.Fatal("UnaryOp(#, unknown) failed")
	}
	assertType(t, got, typ.Integer)

	got, ok = UnaryResult(kind.UnaryNeg, typ.Never)
	if !ok {
		t.Fatal("UnaryOp(-, never) failed")
	}
	assertType(t, got, typ.Never)

	got, ok = UnaryResult(kind.UnaryNot, typ.Unknown)
	if !ok {
		t.Fatal("UnaryOp(not, unknown) failed")
	}
	assertType(t, got, typ.Boolean)
}

func TestOperatorMetamethodPrecedence(t *testing.T) {
	lenRecord := recordWithMetamethod("__len", typ.Func().Returns(typ.String).Build())
	got, ok := UnaryResult(kind.UnaryLen, lenRecord)
	if !ok {
		t.Fatal("UnaryOp(#, record with __len) failed")
	}
	assertType(t, got, typ.String)

	left := recordWithMetamethod("__add", typ.Func().Returns(typ.String).Build())
	right := recordWithMetamethod("__add", typ.Func().Returns(typ.Number).Build())
	got, ok = BinaryResult(left, kind.BinaryAdd, right)
	if !ok {
		t.Fatal("BinaryOp(record __add, +, record __add) failed")
	}
	assertType(t, got, typ.String)

	noAdd := typetable.NewRecord().Build()
	got, ok = BinaryResult(noAdd, kind.BinaryAdd, right)
	if !ok {
		t.Fatal("BinaryOp(record, +, right __add) failed")
	}
	assertType(t, got, typ.Number)
}

func TestOperatorSwappedComparisonMetamethod(t *testing.T) {
	left := typetable.NewRecord().Build()
	right := recordWithMetamethod("__lt", typ.Func().Returns(typ.Boolean).Build())

	got, ok := BinaryResult(left, kind.BinaryGreater, right)
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
