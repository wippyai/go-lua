package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestObjectLiteralViewEvaluatorMarksConstructedValueFresh(t *testing.T) {
	reg := standard.Registry()
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1001), HasExpr: true}
	litID := identity.LuaTableLiteral(7001, 1001)
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("id"), source),
	}).WithIdentity(litID)

	got, ok := objectLiteralViewEvaluator(reg, nil)(lit.View(), factflow.ValueSourceResolverFunc(func(factflow.ValueSource) (product.Value, bool) {
		return typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String), true
	}))
	if !ok {
		t.Fatal("objectLiteralViewEvaluator returned false")
	}
	if gotEscape := product.Get(reg, got, escape.Key); !escape.Equal(gotEscape, escape.Fresh()) {
		t.Fatalf("object literal escape = %s, want fresh", gotEscape)
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); !ok || gotID != litID {
		t.Fatalf("object literal identity = %v/%v, want %v", gotID, ok, litID)
	}
}

func TestObjectLiteralViewEvaluatorMarksEmptyConstructedValueFresh(t *testing.T) {
	reg := standard.Registry()
	litID := identity.LuaTableLiteral(7001, 1002)
	lit := factflow.NewObjectLiteral(nil).WithIdentity(litID)

	got, ok := objectLiteralViewEvaluator(reg, nil)(lit.View(), factflow.ValueSourceResolverFunc(func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	}))
	if !ok {
		t.Fatal("objectLiteralViewEvaluator returned false")
	}
	if gotEscape := product.Get(reg, got, escape.Key); !escape.Equal(gotEscape, escape.Fresh()) {
		t.Fatalf("object literal escape = %s, want fresh", gotEscape)
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); !ok || gotID != litID {
		t.Fatalf("object literal identity = %v/%v, want %v", gotID, ok, litID)
	}
}

func TestObjectLiteralViewEvaluatorUsesExpectedTypeForEmptyConstructor(t *testing.T) {
	reg := standard.Registry()
	want := typetable.NewMap(typ.String, typ.String)
	litID := identity.LuaTableLiteral(7001, 1003)
	lit := factflow.NewObjectLiteral(nil).
		WithIdentity(litID).
		WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, want), want))

	got, ok := objectLiteralViewEvaluator(reg, nil)(lit.View(), factflow.ValueSourceResolverFunc(func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	}))
	if !ok {
		t.Fatal("objectLiteralViewEvaluator returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("object literal type = %v/%v, want %v", gotType, ok, want)
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); !ok || gotID != litID {
		t.Fatalf("object literal identity = %v/%v, want %v", gotID, ok, litID)
	}
}

func TestObjectLiteralViewEvaluatorChildEntriesRefineOverlappingParent(t *testing.T) {
	reg := standard.Registry()
	parentSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1004), HasExpr: true}
	statusSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1005), HasExpr: true}
	headersSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1006), HasExpr: true}
	parentType := typetable.NewRecord().
		Field("status", typ.MaterializeOptional(typ.Integer)).
		Field("headers", typ.MaterializeOptional(typetable.NewMap(typ.String, typ.String))).
		Build()
	headersType := typetable.NewMap(typ.String, typ.String)
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("value"), parentSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("value").Field("status"), statusSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("value").Field("headers"), headersSource),
	})

	got, ok := objectLiteralViewEvaluator(reg, nil)(lit.View(), factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		switch source.ExprRef {
		case parentSource.ExprRef:
			return typevalue.WithWitness(reg, typevalue.FromType(reg, parentType), parentType), true
		case statusSource.ExprRef:
			return typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Integer), typ.Integer), true
		case headersSource.ExprRef:
			return typevalue.WithWitness(reg, typevalue.FromType(reg, headersType), headersType), true
		default:
			return product.Value{}, false
		}
	}))
	if !ok {
		t.Fatal("objectLiteralViewEvaluator returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	want := typetable.NewRecord().
		Field("value", typetable.NewRecord().
			Field("status", typ.Integer).
			Field("headers", headersType).
			Build()).
		Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("object literal type = %v/%v, want %v", gotType, ok, want)
	}
}

func TestExpressionOperationEvaluatorUsesLuaOperationSemantics(t *testing.T) {
	reg := standard.Registry()
	source := factflow.NewNilValueSource(0)
	op, ok := factflow.NewBinaryExpressionOperation("+", source, source)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	first := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(0)), typ.LiteralInt(0))
	second := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(1)), typ.LiteralInt(1))
	left := product.Join(reg, first, second)
	right := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralInt(1)), typ.LiteralInt(1))

	got, ok := expressionOperationEvaluator(reg, nil)(op, left, right)
	if !ok {
		t.Fatal("expressionOperationEvaluator returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Integer) {
		t.Fatalf("operation type = %v/%v, want integer", gotType, ok)
	}
}
