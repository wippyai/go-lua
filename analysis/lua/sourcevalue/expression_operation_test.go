package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
