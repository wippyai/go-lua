package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func ExpressionOperationValue(reg *axis.Registry, typeValues *typevalue.Cache, op factflow.ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
	t, ok := expressionOperationType(reg, op, left, right)
	if !ok {
		return topOriginOperationValue(reg, op, left, right)
	}
	value := typeValues.FromTypeWithWitness(reg, t)
	if typ.IsAny(t) || typ.IsUnknown(t) || op.Op() == "and" || op.Op() == "or" {
		value = inheritOperationTopOrigin(reg, value, left, right)
	}
	return value, true
}

func expressionOperationType(reg *axis.Registry, op factflow.ExpressionOperation, left product.Value, right product.Value) (typ.Type, bool) {
	leftType, ok := operationOperandType(reg, left)
	if !ok {
		return nil, false
	}
	switch op.Kind() {
	case factflow.ExpressionOperationUnary:
		return typeoperator.UnaryOp(op.Op(), leftType)
	case factflow.ExpressionOperationBinary:
		rightType, ok := operationOperandType(reg, right)
		if !ok {
			return nil, false
		}
		return typeoperator.BinaryOp(leftType, op.Op(), rightType)
	default:
		return nil, false
	}
}

func operationOperandType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if t, ok := typevalue.TypeOf(reg, value); ok {
		return t, true
	}
	if ev, ok := topOriginEvidence(reg, value); ok {
		if ev.IsGradualTop() || ev.IsExplicitTop() {
			return typ.Any, true
		}
	}
	return nil, false
}

func topOriginOperationValue(reg *axis.Registry, op factflow.ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
	if op.Op() != "and" && op.Op() != "or" {
		return product.Value{}, false
	}
	ev, ok := operationTopOrigin(reg, left, right)
	if !ok {
		return product.Value{}, false
	}
	return product.Set(reg, product.Top(), evidence.Key, ev), true
}

func inheritOperationTopOrigin(reg *axis.Registry, value, left, right product.Value) product.Value {
	ev, ok := operationTopOrigin(reg, left, right)
	if !ok {
		return value
	}
	return product.Set(reg, value, evidence.Key, ev)
}

func operationTopOrigin(reg *axis.Registry, left, right product.Value) (evidence.Value, bool) {
	leftEvidence, leftOK := topOriginEvidence(reg, left)
	rightEvidence, rightOK := topOriginEvidence(reg, right)
	switch {
	case leftOK && rightOK:
		if evidence.Equal(leftEvidence, rightEvidence) {
			return leftEvidence, true
		}
		return evidence.Top(), false
	case leftOK:
		return leftEvidence, true
	case rightOK:
		return rightEvidence, true
	default:
		return evidence.Top(), false
	}
}

func topOriginEvidence(reg *axis.Registry, value product.Value) (evidence.Value, bool) {
	ev := product.Get(reg, value, evidence.Key)
	if ev.IsGradualTop() || ev.IsExplicitTop() {
		return ev, true
	}
	return evidence.Top(), false
}
