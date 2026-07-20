package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type expressionOperation interface {
	Kind() factflow.ExpressionOperationKind
	Op() string
}

type binaryOperation string

func (op binaryOperation) Kind() factflow.ExpressionOperationKind {
	return factflow.ExpressionOperationBinary
}
func (op binaryOperation) Op() string { return string(op) }

type unaryOperation string

func (op unaryOperation) Kind() factflow.ExpressionOperationKind {
	return factflow.ExpressionOperationUnary
}
func (op unaryOperation) Op() string { return string(op) }

func ExpressionOperationValue(reg *axis.Registry, typeValues *typevalue.Cache, op factflow.ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
	return expressionOperationValue(reg, typeValues, op, left, right)
}

// BinaryOperationValue evaluates a source-independent pure binary operation
// through the same kernel used by ExpressionOperationValue. It exists for
// durable symbolic terms, which retain the operator and operand relations but
// intentionally do not manufacture syntax-shaped ValueSource payloads.
func BinaryOperationValue(reg *axis.Registry, typeValues *typevalue.Cache, operator string, left, right product.Value) (product.Value, bool) {
	if operator == "" {
		return product.Value{}, false
	}
	return expressionOperationValue(reg, typeValues, binaryOperation(operator), left, right)
}

// UnaryOperationValue evaluates a source-independent pure unary operation
// through the same authority as factflow expression operations. Lua not is
// kept literal-exact whenever the operand has definite truthiness.
func UnaryOperationValue(reg *axis.Registry, typeValues *typevalue.Cache, operator string, operand product.Value) (product.Value, bool) {
	if operator != "not" && operator != "#" && operator != "-" && operator != "~" {
		return product.Value{}, false
	}
	if operator == "not" {
		canTruthy := valuerefine.CanBeTruthy(reg, operand)
		canFalsy := valuerefine.CanBeFalsy(reg, operand)
		if canTruthy != canFalsy {
			return typevalue.LiteralBool(reg, canFalsy), true
		}
	}
	return expressionOperationValue(reg, typeValues, unaryOperation(operator), operand, product.Value{})
}

func expressionOperationValue(reg *axis.Registry, typeValues *typevalue.Cache, op expressionOperation, left product.Value, right product.Value) (product.Value, bool) {
	if reg == nil {
		return product.Value{}, false
	}
	// Pure Lua operators are strict in their evaluated operands. Bottom is the
	// exact least abstract result, not a missing evaluator. Logical selectors
	// are the sole exception for their right operand: the left short-circuit arm
	// may survive even while the reached right arm is Bottom.
	if product.Equal(reg, left, product.Bottom(reg)) {
		return product.Bottom(reg), true
	}
	if op.Kind() == factflow.ExpressionOperationBinary &&
		product.Equal(reg, right, product.Bottom(reg)) &&
		op.Op() != "and" && op.Op() != "or" {
		return product.Bottom(reg), true
	}
	if value, ok := exactIdentityComparisonValue(reg, op, left, right); ok {
		return value, true
	}
	if value, ok := exactLiteralComparisonValue(reg, op, left, right); ok {
		return value, true
	}
	// and/or are value selectors, not type-producing operators. Evaluate their
	// abstract arms before the type algebra so raw product Top remains the true
	// lattice upper bound rather than acquiring a narrower Unknown witness.
	if op.Kind() == factflow.ExpressionOperationBinary && (op.Op() == "and" || op.Op() == "or") &&
		product.Equal(reg, left, product.Top()) && product.Equal(reg, right, product.Top()) {
		if value, ok := logicalSelectorValue(reg, op, left, right); ok {
			return refineLogicalOperationValue(reg, op, value, left, right), true
		}
	}
	t, ok := expressionOperationType(reg, typeValues, op, left, right)
	if !ok {
		if value, ok := presentUnknownConcatValue(reg, op, left, right); ok {
			return value, true
		}
		value, ok := topOriginOperationValue(reg, op, left, right)
		if ok {
			return refineLogicalOperationValue(reg, op, value, left, right), true
		}
		// Lua and/or are pure selectors even when neither operand carries a
		// recoverable static type or gradual-top origin.
		if value, ok := logicalSelectorValue(reg, op, left, right); ok {
			return refineLogicalOperationValue(reg, op, value, left, right), true
		}
		return product.Value{}, false
	}
	value := typeValues.FromTypeWithWitness(reg, t)
	if typ.IsAny(t) || typ.IsUnknown(t) {
		value = inheritOperationTopOrigin(reg, value, left, right)
	}
	return refineLogicalOperationValue(reg, op, value, left, right), true
}

func logicalSelectorValue(reg *axis.Registry, op expressionOperation, left, right product.Value) (product.Value, bool) {
	if reg == nil || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != "and" && op.Op() != "or" {
		return product.Value{}, false
	}
	truthy := logicalLeftTruthinessArm(reg, left, true)
	falsy := logicalLeftTruthinessArm(reg, left, false)
	switch op.Op() {
	case "and":
		return product.Join(reg, falsy, logicalReachedArm(reg, truthy, right)), true
	case "or":
		return product.Join(reg, truthy, logicalReachedArm(reg, falsy, right)), true
	}
	return product.Value{}, false
}

// logicalLeftTruthinessArm projects the short-circuit operand onto the
// selected Lua truthiness branch. Presence and runtime kind are the canonical
// product coordinates for nil/falsy exclusion; reducers retain every other
// compatible axis. A branch with no inhabitants is exact Bottom.
func logicalLeftTruthinessArm(reg *axis.Registry, value product.Value, truthy bool) product.Value {
	if truthy {
		if !valuerefine.CanBeTruthy(reg, value) {
			return product.Bottom(reg)
		}
		value = product.WithPresence(reg, value, presence.Present())
		kinds := product.Get(reg, value, runtimekind.Key)
		if !kinds.IsBottom() {
			value = product.Set(reg, value, runtimekind.Key, kinds.Without(runtimekind.Nil))
		}
		return value
	}
	if !valuerefine.CanBeFalsy(reg, value) {
		return product.Bottom(reg)
	}
	kinds := product.Get(reg, value, runtimekind.Key)
	if !kinds.IsBottom() {
		falsyKinds := runtimekind.Join(runtimekind.Singleton(runtimekind.Nil), runtimekind.Singleton(runtimekind.Boolean))
		value = product.Set(reg, value, runtimekind.Key, runtimekind.Meet(kinds, falsyKinds))
	}
	return value
}

func logicalReachedArm(reg *axis.Registry, reach, value product.Value) product.Value {
	if product.Equal(reg, reach, product.Bottom(reg)) {
		return product.Bottom(reg)
	}
	return value
}

func exactIdentityComparisonValue(reg *axis.Registry, op expressionOperation, left, right product.Value) (product.Value, bool) {
	if reg == nil || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != "==" && op.Op() != "~=" {
		return product.Value{}, false
	}
	leftID, leftExact := identityvalue.ExactID(reg, left)
	rightID, rightExact := identityvalue.ExactID(reg, right)
	if !leftExact || !rightExact {
		return product.Value{}, false
	}
	equal := leftID == rightID
	if op.Op() == "~=" {
		equal = !equal
	}
	return typevalue.LiteralBool(reg, equal), true
}

func exactLiteralComparisonValue(reg *axis.Registry, op expressionOperation, left, right product.Value) (product.Value, bool) {
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

func presentUnknownConcatValue(reg *axis.Registry, op expressionOperation, left, right product.Value) (product.Value, bool) {
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
	return refineLogicalOperationValue(reg, op, value, left, right)
}

func refineLogicalOperationValue(reg *axis.Registry, op expressionOperation, value, left, right product.Value) product.Value {
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

func expressionOperationType(reg *axis.Registry, typeValues *typevalue.Cache, op expressionOperation, left product.Value, right product.Value) (typ.Type, bool) {
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
	// A product with every semantic axis unconstrained remains an unknown Lua
	// value after an exact presence refinement.  Requiring byte-equality with
	// product.Top here made `present(Top)` lose the operator algebra that raw Top
	// had, even though refinement can only remove inhabitants.  On a normal
	// continuation, dynamic operators therefore project their ordinary result
	// type from this unknown operand; their existing diagnostic/error effect is
	// owned separately and is not suppressed here.  Definitely absent is nil,
	// not unknown, and remains outside the normal domain of operators such as #.
	if kinds := product.Get(reg, value, runtimekind.Key); kinds.IsTop() {
		switch p := product.PresenceOf(value); {
		case presence.Equal(p, presence.Absent()):
			return typ.Nil, true
		case !p.IsBottom():
			return typ.Unknown, true
		}
	}
	// Raw product Top is the exact unconstrained abstract Lua value. It has no
	// origin evidence to preserve, but it is still a complete operand for the
	// operator algebra: dynamic operators project their normal-result type
	// (for example, # yields integer and ordered comparisons yield boolean).
	if product.Equal(reg, value, product.Top()) {
		return typ.Unknown, true
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

func topOriginOperationValue(reg *axis.Registry, op expressionOperation, left product.Value, right product.Value) (product.Value, bool) {
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
