package transformer

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// evalValueCanonical is the concrete recursion adapter for the same stateless
// node algebra used by guardedValueDecision. State is observed only in the
// slot/dynamic callbacks at this outer edge.
func (a *Arena) evalValueCanonical(term ValueTerm, cursor BindingCursor, context SpecializationContext) (product.Value, bool) {
	return a.evalValueCanonicalWithLeaves(term, a.concreteValueLeafResolver(cursor, context))
}

func (a *Arena) concreteValueLeafResolver(cursor BindingCursor, context SpecializationContext) valueNodeLeafResolver {
	return valueNodeLeafResolver{
		root: func(root Root) (product.Value, bool) {
			if root.Kind == RootMiddle {
				if context.MiddleValue == nil {
					return product.Value{}, false
				}
				return context.MiddleValue(root)
			}
			return cursor.Value(root)
		},
		slot: func(slot key.Value) (product.Value, bool) {
			if !context.HasEnvironment || !a.validEnvironmentSlot(slot) {
				return product.Value{}, false
			}
			return context.Environment.ReadValue(a.reg, slot), true
		},
		cellResult: func(candidate valueNode, values []product.Value) (product.Value, bool) {
			if context.CellResult == nil {
				return product.Value{}, false
			}
			return context.CellResult(candidate.cell, values)
		},
		frameResult: func(candidate valueNode) (product.Value, bool) {
			if context.FrameResult == nil {
				return product.Value{}, false
			}
			return context.FrameResult(candidate.frame, candidate.resultIndex)
		},
		iteratorProjection: func(candidate valueNode, source product.Value) (product.Value, bool) {
			if context.IteratorProjection == nil {
				return product.Value{}, false
			}
			return context.IteratorProjection(candidate.iterator, candidate.variableIndex, source)
		},
		allocationResult: func(candidate valueNode) (product.Value, bool) {
			return a.allocationResult(candidate.allocation, candidate.resultIndex)
		},
	}
}

// evalValueCanonicalWithLeaves is the sole recursive interpreter for a sealed
// ValueTerm DAG. Concrete specialization and formal tuple-leaf specialization
// differ only in their irreducible leaf observations; neither may restate a
// ValueOp. Select correlation, short-circuiting, and every product axis remain
// owned here and in resolveValueNodeProduct.
func (a *Arena) evalValueCanonicalWithLeaves(term ValueTerm, resolver valueNodeLeafResolver) (product.Value, bool) {
	if a == nil || a.reg == nil || term == 0 || int(term) >= len(a.values) {
		return product.Value{}, false
	}
	if resolver.scope != nil {
		resolver = resolver.scope(term, resolver)
	}
	node := a.values[term]
	if node.op == valueSelect {
		if len(node.args) != 2 || node.guard == 0 {
			return product.Value{}, false
		}
		canTrue, canFalse, ok := false, false, false
		if resolver.guard != nil {
			canTrue, canFalse, ok = resolver.guard(node.guard)
		}
		if !ok {
			canTrue, canFalse, ok = a.evalGuardPossibilitiesWithLeaves(node.guard, resolver)
		}
		if !ok || !canTrue && !canFalse {
			return product.Value{}, false
		}
		if !canFalse {
			return a.evalValueCanonicalWithLeaves(node.args[0], resolver)
		}
		if !canTrue {
			return a.evalValueCanonicalWithLeaves(node.args[1], resolver)
		}
		high, highOK := a.evalValueCanonicalWithLeaves(node.args[0], resolver)
		low, lowOK := a.evalValueCanonicalWithLeaves(node.args[1], resolver)
		if !highOK || !lowOK {
			return product.Value{}, false
		}
		return product.Join(a.reg, high, low), true
	}
	args := make([]product.Value, len(node.args))
	for index, child := range node.args {
		value, exact := a.evalValueCanonicalWithLeaves(child, resolver)
		if !exact {
			if node.op == valueExpressionRefinement && index == 0 {
				value, exact = product.Bottom(a.reg), true
			} else {
				return product.Value{}, false
			}
		}
		args[index] = value
		if index == 0 && node.op == valueBinaryOperation {
			if node.operator == "and" && !valuerefine.CanBeTruthyCached(a.reg, a.typeValues, value) || node.operator == "or" && !valuerefine.CanBeFalsyCached(a.reg, a.typeValues, value) {
				return value, true
			}
		}
	}
	return resolveValueNodeProduct(a.reg, a.typeValues, node, args, resolver)
}

func (a *Arena) evalGuardPossibilitiesWithLeaves(guard Guard, resolver valueNodeLeafResolver) (canTrue, canFalse, ok bool) {
	if guard == 0 || int(guard) >= len(a.guards) || a.reg == nil {
		return false, false, false
	}
	n := a.guards[guard]
	switch n.op {
	case guardTrue:
		return true, false, true
	case guardFalse:
		return false, true, true
	case guardTruthy, guardFalsy:
		value, exact := a.evalValueCanonicalWithLeaves(n.value, resolver)
		if !exact {
			return false, false, false
		}
		truthy := valuerefine.CanBeTruthyCached(a.reg, a.typeValues, value)
		falsy := valuerefine.CanBeFalsyCached(a.reg, a.typeValues, value)
		if n.op == guardTruthy {
			return truthy, falsy, true
		}
		return falsy, truthy, true
	case guardAnd:
		canTrue, canFalse = true, false
		for _, arg := range n.args {
			argTrue, argFalse, exact := a.evalGuardPossibilitiesWithLeaves(arg, resolver)
			if !exact {
				return false, false, false
			}
			canTrue = canTrue && argTrue
			canFalse = canFalse || argFalse
		}
		return canTrue, canFalse, true
	case guardOr:
		canTrue, canFalse = false, true
		for _, arg := range n.args {
			argTrue, argFalse, exact := a.evalGuardPossibilitiesWithLeaves(arg, resolver)
			if !exact {
				return false, false, false
			}
			canTrue = canTrue || argTrue
			canFalse = canFalse && argFalse
		}
		return canTrue, canFalse, true
	default:
		return false, false, false
	}
}

// valueNodeLeafResolver is the typed context boundary of the canonical value
// algebra. Implementations supply only irreducible leaf observations; syntax
// and product.Value execution remain owned by resolveValueNodeProduct.
type valueNodeLeafResolver struct {
	// scope selects the carrier wire for one term before its node is
	// interpreted. It changes only irreducible leaf observations; recursion and
	// every ValueOp remain owned by evalValueCanonicalWithLeaves.
	scope func(ValueTerm, valueNodeLeafResolver) valueNodeLeafResolver
	// guard optionally supplies an exact region-conditioned Boolean result.
	// The canonical evaluator still owns Select traversal; formal execution
	// merely observes its already-compiled ROBDD region at this leaf.
	guard              func(Guard) (canTrue, canFalse, exact bool)
	root               func(Root) (product.Value, bool)
	slot               func(key.Value) (product.Value, bool)
	cellResult         func(valueNode, []product.Value) (product.Value, bool)
	frameResult        func(valueNode) (product.Value, bool)
	dynamicRead        func(valueNode, []product.Value) (product.Value, bool)
	iteratorProjection func(valueNode, product.Value) (product.Value, bool)
	allocationResult   func(valueNode) (product.Value, bool)
	// completeImpossibleConcat preserves a formally completed leaf whose concat
	// has no normal continuation. The concrete evaluator leaves it unsupported;
	// a formal tuple leaf may materialize bottom so its diagnostic survives
	// without publishing a string result.
	completeImpossibleConcat func() (product.Value, bool)
}

// resolveValueNodeProduct is the single stateless per-node value semantics.
// Children are already resolved in node.args order. Select correlation and
// short-circuit demand are traversal concerns and are intentionally handled by
// the concrete or decision-DAG caller before entering this algebra.
func resolveValueNodeProduct(reg *axis.Registry, typeValues *typevalue.Cache, node valueNode, args []product.Value, resolver valueNodeLeafResolver) (product.Value, bool) {
	if reg == nil || len(args) != len(node.args) {
		return product.Value{}, false
	}
	bottom := product.Bottom(reg)
	switch node.op {
	case valueRoot:
		if resolver.root == nil {
			return product.Value{}, false
		}
		return resolver.root(node.root)
	case valueEnvironment:
		if resolver.slot == nil || node.slot == 0 {
			return product.Value{}, false
		}
		return resolver.slot(node.slot)
	case valueConstant:
		return node.value, product.BelongsToRegistry(reg, node.value)
	case valueObjectLiteral:
		if !node.objectPlan.Valid() || node.objectPlan.ValueSourceCount() != len(args) {
			return product.Value{}, false
		}
		row := make([]luasourcevalue.ObjectLiteralPlanValue, len(args))
		for index, value := range args {
			row[index] = luasourcevalue.ObjectLiteralPlanValue{Value: value, Available: true}
		}
		return luasourcevalue.ComposeObjectLiteralPlanCached(reg, nil, node.objectPlan, row)
	case valueJoin:
		out := bottom
		for _, value := range args {
			out = product.Join(reg, out, value)
		}
		return out, true
	case valueRefinement:
		if len(args) != 1 {
			return product.Value{}, false
		}
		return factapply.RefineProductValueConstraint(reg, args[0], node.value), true
	case valueFalsyAbsentRefinement:
		if len(args) != 1 {
			return product.Value{}, false
		}
		if valuerefine.CanBeFalse(reg, args[0]) {
			return args[0], true
		}
		return factapply.RefineProductValueConstraint(reg, args[0], node.value), true
	case valueExpressionRefinement:
		if len(args) != 1 {
			return product.Value{}, false
		}
		return enginesourcevalue.ApplyExpressionRefinement(reg, args[0], node.expressionRefinement()), true
	case valueCellResult:
		if resolver.cellResult == nil {
			return product.Value{}, false
		}
		return resolver.cellResult(node, args)
	case valueCallResult:
		if len(args) != 1 {
			return product.Value{}, false
		}
		return args[0], true
	case valuePredicateObservation:
		if len(args) != 1 {
			return product.Value{}, false
		}
		return args[0], true
	case valueFrameResult:
		if resolver.frameResult == nil {
			return product.Value{}, false
		}
		return resolver.frameResult(node)
	case valueDynamicRead, valueDynamicTableRead:
		// Dynamic reads are nonseparable registered-factor queries. The shared
		// node algebra owns dispatch; concrete and formal carriers provide the
		// same irreducible evidence observation through this callback.
		if resolver.dynamicRead == nil {
			return product.Value{}, false
		}
		return resolver.dynamicRead(node, args)
	case valueStringConcat:
		if len(args) != 2 {
			return product.Value{}, false
		}
		if exactConcatOperand(reg, args[0]) && exactConcatOperand(reg, args[1]) {
			if value, exact := luasourcevalue.BinaryOperationValue(reg, nil, "..", args[0], args[1]); exact {
				return value, true
			}
		}
		// A conservative string result is valid only for a possibly normal
		// continuation. Invalid operands may still produce a diagnostic, but
		// they do not publish a normal concat result.
		if !possibleConcatOperand(reg, args[0]) || !possibleConcatOperand(reg, args[1]) {
			if resolver.completeImpossibleConcat != nil {
				return resolver.completeImpossibleConcat()
			}
			return product.Value{}, false
		}
		return typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String), true
	case valueUnaryOperation:
		if len(args) != 1 || !isPureUnaryOperator(node.operator) {
			return product.Value{}, false
		}
		return luasourcevalue.UnaryOperationValue(reg, nil, node.operator, args[0])
	case valueBinaryOperation:
		if len(args) != 2 || !isPureBinaryOperator(node.operator) {
			return product.Value{}, false
		}
		if node.operator == "and" && !valuerefine.CanBeTruthyCached(reg, typeValues, args[0]) || node.operator == "or" && !valuerefine.CanBeFalsyCached(reg, typeValues, args[0]) {
			return args[0], true
		}
		return luasourcevalue.BinaryOperationValue(reg, nil, node.operator, args[0], args[1])
	case valueIteratorProjection:
		if len(args) < 1 || len(args) > 2 {
			return product.Value{}, false
		}
		if product.Equal(reg, args[0], bottom) {
			return bottom, true
		}
		if resolver.iteratorProjection != nil {
			if value, exact := resolver.iteratorProjection(node, args[0]); exact {
				return value, true
			}
		}
		if value, exact := luasourcevalue.IteratorVariableValue(reg, nil, node.iterator, node.variableIndex, args[0], node.assertedType, node.hasAsserted); exact {
			return value, true
		}
		if len(args) == 2 {
			return args[1], true
		}
		return product.Value{}, false
	case valueGenericForResult:
		if len(args) != 4 {
			return product.Value{}, false
		}
		if value, exact := luasourcevalue.GenericForProtocolResult(reg, nil, node.variableIndex, args[0]); exact {
			return value, true
		}
		return args[3], true
	case valueLoopContinuation:
		if len(args) != 0 || node.owner == (lexicalidentity.StableLexicalBodyID{}) {
			return product.Value{}, false
		}
		return typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean), true
	case valueStaticIndex:
		if len(args) != 2 {
			return product.Value{}, false
		}
		return enginesourcevalue.StaticIndexValue(reg, nil, args[0], args[1])
	case valueAllocationResult:
		if resolver.allocationResult == nil {
			return product.Value{}, false
		}
		return resolver.allocationResult(node)
	case valueLuaTypeName:
		if len(args) != 1 {
			return product.Value{}, false
		}
		return enginesourcevalue.LuaTypeNameValue(reg, nil, args[0])
	default:
		return product.Value{}, false
	}
}
