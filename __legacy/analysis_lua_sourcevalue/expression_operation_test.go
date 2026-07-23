package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestExpressionOperationLogicalPreservesTopOriginEvidence(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("or", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	left := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	right := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.GradualTop()) {
		t.Fatalf("logical operation evidence = %s, want %s", gotEvidence, evidence.GradualTop())
	}
}

func TestBinaryLogicalSelectorLatticeArms(t *testing.T) {
	reg := standard.Registry()
	objectType := typetable.BuiltinTopMarker()
	object := typevalue.WithWitness(reg, typevalue.FromType(reg, objectType), objectType)
	truthy := typevalue.LiteralString(reg, "kept")
	falsy := typevalue.Nil(reg)

	got, ok := BinaryOperationValue(reg, nil, "or", truthy, object)
	if !ok || !product.Equal(reg, got, truthy) {
		t.Fatalf("truthy or object = %#v/%t, want left arm", got, ok)
	}
	got, ok = BinaryOperationValue(reg, nil, "or", falsy, object)
	if !ok || !product.Equal(reg, got, object) {
		t.Fatalf("falsy or object = %#v/%t, want right arm", got, ok)
	}
	got, ok = BinaryOperationValue(reg, nil, "or", product.Top(), object)
	if !ok {
		t.Fatal("mixed or object was not an exact lattice transfer")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("mixed or object presence = %s, want present", gotPresence)
	}
	if gotKinds := product.Get(reg, got, runtimekind.Key); gotKinds.Contains(runtimekind.Nil) {
		t.Fatalf("mixed or object runtime kinds = %s, want truthy-left/right projection without nil", gotKinds)
	}
	got, ok = BinaryOperationValue(reg, nil, "and", product.Bottom(reg), object)
	if !ok || !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("bottom and object = %#v/%t, want bottom", got, ok)
	}
}

func TestExpressionOperationEqualityOfExactLiteralsReturnsBooleanLiteral(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	eq, ok := factflow.NewBinaryExpressionOperation("==", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(==) returned false")
	}
	ne, ok := factflow.NewBinaryExpressionOperation("~=", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(~=) returned false")
	}
	left := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("string")), typ.LiteralString("string"))
	same := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("string")), typ.LiteralString("string"))
	other := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("boolean")), typ.LiteralString("boolean"))

	assertLiteralBool := func(label string, got product.Value, ok bool, want typ.Type) {
		t.Helper()
		if !ok {
			t.Fatalf("%s returned false", label)
		}
		gotType, typeOK := typevalue.TypeOf(reg, got)
		if !typeOK || !typ.TypeEquals(gotType, want) {
			t.Fatalf("%s type = %v/%v, want %v", label, gotType, typeOK, want)
		}
	}

	got, ok := ExpressionOperationValue(reg, nil, eq, left, same)
	assertLiteralBool("equal same", got, ok, typ.True)
	got, ok = ExpressionOperationValue(reg, nil, eq, left, other)
	assertLiteralBool("equal other", got, ok, typ.False)
	got, ok = ExpressionOperationValue(reg, nil, ne, left, other)
	assertLiteralBool("not equal other", got, ok, typ.True)
}

func TestExpressionOperationEqualityWithUnreadableOperandStillReturnsBoolean(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("==", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	left := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	right := product.Top()

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	gotType, typeOK := typevalue.TypeOf(reg, got)
	if !typeOK || !typ.TypeEquals(gotType, typ.Boolean) {
		t.Fatalf("equality type = %v/%v, want boolean", gotType, typeOK)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); gotEvidence.IsExplicitTop() || gotEvidence.IsGradualTop() {
		t.Fatalf("equality evidence = %s, want operator proof not top-origin", gotEvidence)
	}
}

func TestExpressionOperationNotWithUnreadableOperandStillReturnsBoolean(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewUnaryExpressionOperation("not", source)
	if !ok {
		t.Fatal("NewUnaryExpressionOperation returned false")
	}

	got, ok := ExpressionOperationValue(reg, nil, op, product.Top(), product.Value{})
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	gotType, typeOK := typevalue.TypeOf(reg, got)
	if !typeOK || !typ.TypeEquals(gotType, typ.Boolean) {
		t.Fatalf("not type = %v/%v, want boolean", gotType, typeOK)
	}
}

func TestUnaryLengthOfUnconstrainedOperandHasExactNormalResult(t *testing.T) {
	reg := standard.Registry()
	got, ok := UnaryOperationValue(reg, nil, "#", product.Top())
	if !ok {
		t.Fatal("UnaryOperationValue(# Top) returned false")
	}
	gotType, typeOK := typevalue.TypeOf(reg, got)
	if !typeOK || !typ.TypeEquals(gotType, typ.Integer) {
		t.Fatalf("# Top type = %v/%v, want integer normal result", gotType, typeOK)
	}
}

func TestExpressionOperationOrWithPresentFallbackProvesNonNilWithoutLaunderingUnknown(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("or", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	left := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Unknown), typ.Unknown)
	right := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("fallback")), typ.LiteralString("fallback"))

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("logical or presence = %s, want present", gotPresence)
	}
	if gotKinds := product.Get(reg, got, runtimekind.Key); gotKinds.Contains(runtimekind.Nil) {
		t.Fatalf("logical or runtime kinds = %s, want nil excluded", gotKinds)
	}
	if gotType, ok := typevalue.TypeOf(reg, got); ok && typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("logical or type = %v, want no string laundering from unknown", gotType)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("logical or evidence = %s, want explicit top", gotEvidence)
	}
}

func TestExpressionOperationOrWithPresentFallbackProvesNonNilWithoutLaunderingAny(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("or", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	left := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Any), typ.Any)
	right := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("fallback")), typ.LiteralString("fallback"))

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("logical or presence = %s, want present", gotPresence)
	}
	if gotKinds := product.Get(reg, got, runtimekind.Key); gotKinds.Contains(runtimekind.Nil) {
		t.Fatalf("logical or runtime kinds = %s, want nil excluded", gotKinds)
	}
	if gotType, ok := typevalue.TypeOf(reg, got); ok && typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("logical or type = %v, want no string laundering from any", gotType)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("logical or evidence = %s, want explicit top", gotEvidence)
	}
}

func TestExpressionOperationGuardedScalarOrNilKeepsValidatedOptionalType(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	andOp, ok := factflow.NewBinaryExpressionOperation("and", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(and) returned false")
	}
	orOp, ok := factflow.NewBinaryExpressionOperation("or", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(or) returned false")
	}
	condition := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean)
	validated := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	left, ok := ExpressionOperationValue(reg, nil, andOp, condition, validated)
	if !ok {
		t.Fatal("guarded and value did not resolve")
	}
	nilValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Nil), typ.Nil)
	got, ok := ExpressionOperationValue(reg, nil, orOp, left, nilValue)
	if !ok {
		t.Fatal("guarded fallback value did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	want := typ.MaterializeOptional(typ.String)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("guarded fallback type = %v/%v, want %v", gotType, ok, want)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); gotEvidence.IsExplicitTop() || gotEvidence.IsGradualTop() {
		t.Fatalf("guarded fallback evidence = %s, want concrete proof", gotEvidence)
	}
}

func TestExpressionOperationLogicalOrOfFalseOrStringWithStringKeepsStringWitness(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("or", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(or) returned false")
	}
	leftType := typeexpr.Union(typ.False, typ.LiteralString("ok"))
	left := typevalue.WithWitness(reg, typevalue.FromType(reg, leftType), leftType)
	right := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("logical fallback witness = %v/%v, want string", gotType, ok)
	}

	cache := typevalue.NewCache()
	left = cache.FromTypeWithWitness(reg, leftType)
	right = cache.FromTypeWithWitness(reg, typ.String)
	got, ok = ExpressionOperationValue(reg, cache, op, left, right)
	if !ok {
		t.Fatal("cached ExpressionOperationValue returned false")
	}
	gotType, ok = typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("cached logical fallback witness = %v/%v, want string", gotType, ok)
	}
}

func TestExpressionOperationLogicalSkipDoesNotInheritSkippedTopOriginEvidence(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("or", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	leftType := typ.LiteralBool(true)
	left := typevalue.WithWitness(reg, typevalue.FromType(reg, leftType), leftType)
	right := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, leftType) {
		t.Fatalf("logical skip type = %v/%v, want %v", gotType, ok, leftType)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); gotEvidence.IsExplicitTop() || gotEvidence.IsGradualTop() {
		t.Fatalf("logical skip evidence = %s, want no skipped top-origin evidence", gotEvidence)
	}
}

func TestExpressionOperationLogicalFallbackPreservesTopOriginEvidence(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("and", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	left := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	right := product.Top()

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("logical fallback evidence = %s, want %s", gotEvidence, evidence.ExplicitTop())
	}
}

func TestExpressionOperationDynamicArithmeticPreservesTopOriginEvidence(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("+", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	left := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	right := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(1)), typ.LiteralInt(1))

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.GradualTop()) {
		t.Fatalf("dynamic arithmetic evidence = %s, want %s", gotEvidence, evidence.GradualTop())
	}
}

func TestExpressionOperationConcatWithPresentStructuralOperandReturnsPresentUnknown(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("..", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	errorType := typ.NewInterface("Error", nil)
	left := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	right := typevalue.WithWitness(reg, typevalue.FromType(reg, errorType), errorType)

	got, ok := ExpressionOperationValue(reg, nil, op, left, right)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("concat presence = %s, want present", gotPresence)
	}
	if gotType, ok := typevalue.TypeOf(reg, got); ok && typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("concat type = %v, want no string proof for structural operand", gotType)
	}
}

func TestExpressionOperationJoinedIntegerCounterStaysInteger(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("+", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	first := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(0)), typ.LiteralInt(0))
	second := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(1)), typ.LiteralInt(1))
	counter := product.Join(reg, first, second)
	one := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(1)), typ.LiteralInt(1))

	got, ok := ExpressionOperationValue(reg, nil, op, counter, one)
	if !ok {
		t.Fatal("ExpressionOperationValue returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Integer) {
		t.Fatalf("counter + 1 type = %v/%v, want integer", gotType, ok)
	}
}

func TestExpressionOperationLengthOfBuiltinTableMarkerComparesAsBoolean(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	lenOp, ok := factflow.NewUnaryExpressionOperation("#", source)
	if !ok {
		t.Fatal("NewUnaryExpressionOperation returned false")
	}
	tableValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typetable.BuiltinTopMarker()), typetable.BuiltinTopMarker())

	lengthValue, ok := ExpressionOperationValue(reg, nil, lenOp, tableValue, product.Value{})
	if !ok {
		t.Fatal("ExpressionOperationValue(#table) returned false")
	}
	lengthType, ok := typevalue.TypeOf(reg, lengthValue)
	if !ok || !typ.TypeEquals(lengthType, typ.Integer) {
		t.Fatalf("#table type = %v/%v, want integer", lengthType, ok)
	}

	gtOp, ok := factflow.NewBinaryExpressionOperation(">", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	zero := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(0)), typ.LiteralInt(0))
	got, ok := ExpressionOperationValue(reg, nil, gtOp, lengthValue, zero)
	if !ok {
		t.Fatal("ExpressionOperationValue(#table > 0) returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Boolean) {
		t.Fatalf("#table > 0 type = %v/%v, want boolean", gotType, ok)
	}
}

func TestExpressionOperationLengthOfRuntimeKindTableComparesAsBoolean(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	lenOp, ok := factflow.NewUnaryExpressionOperation("#", source)
	if !ok {
		t.Fatal("NewUnaryExpressionOperation returned false")
	}
	tableValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	tableValue = product.Set(reg, tableValue, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))

	lengthValue, ok := ExpressionOperationValue(reg, nil, lenOp, tableValue, product.Value{})
	if !ok {
		t.Fatal("ExpressionOperationValue(#runtime-table) returned false")
	}
	lengthType, ok := typevalue.TypeOf(reg, lengthValue)
	if !ok || !typ.TypeEquals(lengthType, typ.Integer) {
		t.Fatalf("#runtime-table type = %v/%v, want integer", lengthType, ok)
	}

	gtOp, ok := factflow.NewBinaryExpressionOperation(">", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	zero := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(0)), typ.LiteralInt(0))
	got, ok := ExpressionOperationValue(reg, nil, gtOp, lengthValue, zero)
	if !ok {
		t.Fatal("ExpressionOperationValue(#runtime-table > 0) returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Boolean) {
		t.Fatalf("#runtime-table > 0 type = %v/%v, want boolean", gotType, ok)
	}
}

func TestExpressionOperationLengthOfRuntimeKindTableOverridesAnyOrigin(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	lenOp, ok := factflow.NewUnaryExpressionOperation("#", source)
	if !ok {
		t.Fatal("NewUnaryExpressionOperation returned false")
	}
	tableValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Any), typ.Any)
	tableValue = product.WithPresence(reg, tableValue, presence.Present())
	tableValue = product.Set(reg, tableValue, evidence.Key, evidence.GradualTop())
	tableValue = product.Set(reg, tableValue, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	if got := product.Get(reg, tableValue, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("runtime kind = %s, want table", got)
	}
	operandType, ok := operationOperandType(reg, nil, tableValue)
	if !ok || !typ.TypeEquals(operandType, typetable.BuiltinTopMarker()) {
		t.Fatalf("operation operand type = %v/%v, want builtin table marker", operandType, ok)
	}

	lengthValue, ok := ExpressionOperationValue(reg, nil, lenOp, tableValue, product.Value{})
	if !ok {
		t.Fatal("ExpressionOperationValue(#runtime-table-any) returned false")
	}
	lengthType, ok := typevalue.TypeOf(reg, lengthValue)
	if !ok || !typ.TypeEquals(lengthType, typ.Integer) {
		t.Fatalf("#runtime-table-any type = %v/%v, want integer", lengthType, ok)
	}
}

func TestExpressionOperationLengthPreservesSparseTopPresencePolarity(t *testing.T) {
	reg := standard.Registry()
	presentAny := product.WithPresence(reg, product.Top(), presence.Present())
	length, exact := UnaryOperationValue(reg, nil, "#", presentAny)
	if !exact {
		t.Fatal("# present(Top) lost its normal-result algebra")
	}
	lengthType, typed := typevalue.TypeOf(reg, length)
	if !typed || !typ.TypeEquals(lengthType, typ.Integer) {
		t.Fatalf("# present(Top) type = %v/%v, want integer", lengthType, typed)
	}

	absent := product.WithPresence(reg, product.Top(), presence.Absent())
	if _, exact := UnaryOperationValue(reg, nil, "#", absent); exact {
		t.Fatal("# absent(Top) was treated as a normal unknown result")
	}
}
