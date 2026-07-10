package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func ExpressionOperationValue(reg *axis.Registry, typeValues *typevalue.Cache, op factflow.ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
	if value, ok := exactLiteralComparisonValue(reg, op, left, right); ok {
		return value, true
	}
	t, ok := expressionOperationType(reg, typeValues, op, left, right)
	if !ok {
		if value, ok := presentUnknownConcatValue(reg, op, left, right); ok {
			return value, true
		}
		value, ok := topOriginOperationValue(reg, op, left, right)
		if !ok {
			return product.Value{}, false
		}
		return RefineLogicalOperationValue(reg, op, value, left, right), true
	}
	value := typeValues.FromTypeWithWitness(reg, t)
	if typ.IsAny(t) || typ.IsUnknown(t) {
		value = inheritOperationTopOrigin(reg, value, left, right)
	}
	return RefineLogicalOperationValue(reg, op, value, left, right), true
}

func exactLiteralComparisonValue(reg *axis.Registry, op factflow.ExpressionOperation, left, right product.Value) (product.Value, bool) {
	if reg == nil || op.Kind() != factflow.ExpressionOperationBinary {
		return product.Value{}, false
	}
	equals := false
	switch op.Op() {
	case "==":
		equals = true
	case "~=":
		equals = false
	default:
		return product.Value{}, false
	}
	leftType, leftOK := typevalue.WitnessOf(reg, left)
	rightType, rightOK := typevalue.WitnessOf(reg, right)
	if !leftOK || !rightOK {
		return product.Value{}, false
	}
	leftLit, leftIsLiteral := leftType.(*typ.Literal)
	rightLit, rightIsLiteral := rightType.(*typ.Literal)
	if !leftIsLiteral || !rightIsLiteral {
		return product.Value{}, false
	}
	matched := typ.TypeEquals(leftLit, rightLit)
	if !equals {
		matched = !matched
	}
	lit := typ.LiteralBool(matched)
	return typevalue.WithWitness(reg, typevalue.FromType(reg, lit), lit), true
}

func presentUnknownConcatValue(reg *axis.Registry, op factflow.ExpressionOperation, left, right product.Value) (product.Value, bool) {
	if reg == nil || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != ".." {
		return product.Value{}, false
	}
	if !presence.Equal(product.PresenceOf(left), presence.Present()) ||
		!presence.Equal(product.PresenceOf(right), presence.Present()) {
		return product.Value{}, false
	}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	return inheritOperationTopOrigin(reg, value, left, right), true
}

// RefineLogicalOperationValue applies Lua short-circuit truthiness facts that are
// expressible on the product value but must not rewrite the static type witness.
func RefineLogicalOperationValue(reg *axis.Registry, op factflow.ExpressionOperation, value, left, right product.Value) product.Value {
	if reg == nil || op.Kind() != factflow.ExpressionOperationBinary {
		return value
	}
	if op.Op() != "or" || !presence.Equal(product.PresenceOf(right), presence.Present()) {
		return value
	}
	return provePresentNonNil(reg, value)
}

func provePresentNonNil(reg *axis.Registry, value product.Value) product.Value {
	value = product.WithPresence(reg, value, presence.Present())
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.Contains(runtimekind.Nil) {
		value = product.Set(reg, value, runtimekind.Key, kinds.Without(runtimekind.Nil))
	}
	return value
}

func expressionOperationType(reg *axis.Registry, typeValues *typevalue.Cache, op factflow.ExpressionOperation, left product.Value, right product.Value) (typ.Type, bool) {
	if op.Kind() == factflow.ExpressionOperationBinary && (op.Op() == "==" || op.Op() == "~=") {
		return typ.Boolean, true
	}
	if op.Kind() == factflow.ExpressionOperationUnary && op.Op() == "not" {
		return typ.Boolean, true
	}
	leftType, ok := operationOperandType(reg, typeValues, left)
	if !ok {
		return nil, false
	}
	switch op.Kind() {
	case factflow.ExpressionOperationUnary:
		return typeoperator.UnaryOp(op.Op(), leftType)
	case factflow.ExpressionOperationBinary:
		rightType, ok := operationOperandType(reg, typeValues, right)
		if !ok {
			return nil, false
		}
		return typeoperator.BinaryOp(leftType, op.Op(), rightType)
	default:
		return nil, false
	}
}

func operationOperandType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (typ.Type, bool) {
	runtimeKindType, hasRuntimeKindType := runtimeKindValueType(reg, value)
	if t, ok := typeValues.TypeOf(reg, value); ok {
		if typ.IsAny(t) || typ.IsUnknown(t) {
			if hasRuntimeKindType {
				return runtimeKindType, true
			}
		}
		return t, true
	}
	if hasRuntimeKindType {
		return runtimeKindType, true
	}
	if ev, ok := topOriginEvidence(reg, value); ok {
		if ev.IsGradualTop() || ev.IsExplicitTop() {
			return typ.Any, true
		}
	}
	return nil, false
}

func runtimeKindValueType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	var members []typ.Type
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil:
			members = append(members, typ.Nil)
		case runtimekind.Boolean:
			members = append(members, typ.Boolean)
		case runtimekind.Number:
			members = append(members, typ.Number)
		case runtimekind.String:
			members = append(members, typ.String)
		case runtimekind.Table:
			members = append(members, typetable.BuiltinTopMarker())
		case runtimekind.Function:
			members = append(members, typ.Func().Build())
		default:
			return nil, false
		}
	}
	if len(members) == 0 {
		return nil, false
	}
	t := normalize.UnionForEvidence(members...)
	switch p := product.PresenceOf(value); {
	case presence.Equal(p, presence.Absent()):
		return typ.Nil, true
	case presence.Equal(p, presence.Maybe()):
		if !typevalue.TypeIncludesNil(t) {
			t = normalize.Optional(t)
		}
	}
	return t, true
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
