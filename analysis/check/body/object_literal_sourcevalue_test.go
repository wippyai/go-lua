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

func TestObjectLiteralEvaluatorMarksConstructedValueFresh(t *testing.T) {
	reg := standard.Registry()
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1001), HasExpr: true}
	litID := identity.LuaTableLiteral(7001, 1001)
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("id"), source),
	}).WithIdentity(litID)

	got, ok := objectLiteralEvaluator(reg, nil)(lit, func(factflow.ValueSource) (product.Value, bool) {
		return typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String), true
	})
	if !ok {
		t.Fatal("objectLiteralEvaluator returned false")
	}
	if gotEscape := product.Get(reg, got, escape.Key); !escape.Equal(gotEscape, escape.Fresh()) {
		t.Fatalf("object literal escape = %s, want fresh", gotEscape)
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); !ok || gotID != litID {
		t.Fatalf("object literal identity = %v/%v, want %v", gotID, ok, litID)
	}
}

func TestObjectLiteralEvaluatorMarksEmptyConstructedValueFresh(t *testing.T) {
	reg := standard.Registry()
	litID := identity.LuaTableLiteral(7001, 1002)
	lit := factflow.NewObjectLiteral(nil).WithIdentity(litID)

	got, ok := objectLiteralEvaluator(reg, nil)(lit, func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	})
	if !ok {
		t.Fatal("objectLiteralEvaluator returned false")
	}
	if gotEscape := product.Get(reg, got, escape.Key); !escape.Equal(gotEscape, escape.Fresh()) {
		t.Fatalf("object literal escape = %s, want fresh", gotEscape)
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); !ok || gotID != litID {
		t.Fatalf("object literal identity = %v/%v, want %v", gotID, ok, litID)
	}
}

func TestObjectLiteralEvaluatorUsesExpectedTypeForEmptyConstructor(t *testing.T) {
	reg := standard.Registry()
	want := typetable.NewMap(typ.String, typ.String)
	litID := identity.LuaTableLiteral(7001, 1003)
	lit := factflow.NewObjectLiteral(nil).
		WithIdentity(litID).
		WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, want), want))

	got, ok := objectLiteralEvaluator(reg, nil)(lit, func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	})
	if !ok {
		t.Fatal("objectLiteralEvaluator returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("object literal type = %v/%v, want %v", gotType, ok, want)
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); !ok || gotID != litID {
		t.Fatalf("object literal identity = %v/%v, want %v", gotID, ok, litID)
	}
}
