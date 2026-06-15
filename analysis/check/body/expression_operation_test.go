package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
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

	got, ok := expressionOperationEvaluator(reg)(op, left, right)
	if !ok {
		t.Fatal("expressionOperationEvaluator returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.GradualTop()) {
		t.Fatalf("logical operation evidence = %s, want %s", gotEvidence, evidence.GradualTop())
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

	got, ok := expressionOperationEvaluator(reg)(op, counter, one)
	if !ok {
		t.Fatal("expressionOperationEvaluator returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Integer) {
		t.Fatalf("counter + 1 type = %v/%v, want integer", gotType, ok)
	}
}
