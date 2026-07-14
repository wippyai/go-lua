package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

// evaluatedTermEvaluator is the only executable-term evaluator admitted by
// the shadow evaluated-root slice. It is callback-free, memoizes each DAG node
// once, and checks cancellation during traversal. A failure publishes nothing.
type evaluatedTermEvaluator struct {
	ctx    context.Context
	arena  *Arena
	cursor BindingCursor

	values        map[ValueTerm]product.Value
	valueDone     map[ValueTerm]bool
	valueVisiting map[ValueTerm]bool
	guards        map[Guard]bool
	guardDone     map[Guard]bool
	guardVisiting map[Guard]bool
	steps         uint64
}

func newEvaluatedTermEvaluator(ctx context.Context, arena *Arena, cursor BindingCursor) (*evaluatedTermEvaluator, error) {
	if ctx == nil || arena == nil || arena.reg == nil {
		return nil, fmt.Errorf("transformer: evaluated term evaluator requires context and arena")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &evaluatedTermEvaluator{
		ctx: ctx, arena: arena, cursor: cursor,
		values: make(map[ValueTerm]product.Value), valueDone: make(map[ValueTerm]bool), valueVisiting: make(map[ValueTerm]bool),
		guards: make(map[Guard]bool), guardDone: make(map[Guard]bool), guardVisiting: make(map[Guard]bool),
	}, nil
}

func (e *evaluatedTermEvaluator) checkpoint() error {
	e.steps++
	if e.steps == 1 || e.steps&63 == 0 {
		return e.ctx.Err()
	}
	return nil
}

func (e *evaluatedTermEvaluator) value(term ValueTerm) (product.Value, error) {
	if e.valueDone[term] {
		return e.values[term], nil
	}
	if err := e.checkpoint(); err != nil {
		return product.Value{}, err
	}
	if term == 0 || int(term) >= len(e.arena.values) {
		return product.Value{}, fmt.Errorf("transformer: invalid evaluated value term %d", term)
	}
	if e.valueVisiting[term] {
		return product.Value{}, fmt.Errorf("transformer: cyclic evaluated value term %d", term)
	}
	e.valueVisiting[term] = true
	defer delete(e.valueVisiting, term)

	node := e.arena.values[term]
	var out product.Value
	switch node.op {
	case valueRoot:
		value, ok := e.cursor.Value(node.root)
		if !ok {
			return product.Value{}, fmt.Errorf("transformer: unbound evaluated root")
		}
		out = value
	case valueConstant:
		out = node.value
	case valueJoin:
		out = product.Bottom(e.arena.reg)
		for _, argument := range node.args {
			value, err := e.value(argument)
			if err != nil {
				return product.Value{}, err
			}
			out = product.Join(e.arena.reg, out, value)
		}
	case valueRefinement, valueRuntimeValidation, valueLuaTypeName, valueCallResult:
		if len(node.args) != 1 {
			return product.Value{}, fmt.Errorf("transformer: malformed unary evaluated term")
		}
		value, err := e.value(node.args[0])
		if err != nil {
			return product.Value{}, err
		}
		switch node.op {
		case valueRefinement:
			out = factapply.RefineProductValueConstraint(e.arena.reg, value, node.value)
		case valueRuntimeValidation:
			refinement := factflow.NewExpressionRuntimeValidation(factflow.ValueSource{}, node.value)
			out = enginesourcevalue.ApplyExpressionRefinement(e.arena.reg, value, refinement)
		case valueLuaTypeName:
			var ok bool
			out, ok = enginesourcevalue.LuaTypeNameValue(e.arena.reg, nil, value)
			if !ok {
				return product.Value{}, fmt.Errorf("transformer: Lua type-name evaluation failed")
			}
		case valueCallResult:
			out = value
		}
	case valueStringConcat, valueScalarEqual, valueScalarNotEqual, valueScalarAnd, valueScalarOr, valueStaticIndex:
		if len(node.args) != 2 {
			return product.Value{}, fmt.Errorf("transformer: malformed binary evaluated term")
		}
		left, err := e.value(node.args[0])
		if err != nil {
			return product.Value{}, err
		}
		if node.op == valueScalarAnd && !valuerefine.CanBeTruthy(e.arena.reg, left) {
			out = left
			break
		}
		if node.op == valueScalarOr && !valuerefine.CanBeFalsy(e.arena.reg, left) {
			out = left
			break
		}
		right, err := e.value(node.args[1])
		if err != nil {
			return product.Value{}, err
		}
		switch node.op {
		case valueStringConcat:
			if !exactStringOperand(e.arena.reg, left) || !exactStringOperand(e.arena.reg, right) {
				return product.Value{}, fmt.Errorf("transformer: inexact string-concat operand")
			}
			var ok bool
			out, ok = luasourcevalue.BinaryOperationValue(e.arena.reg, nil, "..", left, right)
			if !ok {
				return product.Value{}, fmt.Errorf("transformer: string concatenation failed")
			}
		case valueStaticIndex:
			var ok bool
			out, ok = enginesourcevalue.StaticIndexValue(e.arena.reg, nil, left, right)
			if !ok {
				return product.Value{}, fmt.Errorf("transformer: static index evaluation failed")
			}
		default:
			operator, ok := scalarBinaryOperator(node.op)
			if !ok {
				return product.Value{}, fmt.Errorf("transformer: unsupported scalar operation")
			}
			out, ok = luasourcevalue.BinaryOperationValue(e.arena.reg, nil, operator, left, right)
			if !ok {
				return product.Value{}, fmt.Errorf("transformer: scalar operation failed")
			}
		}
	default:
		return product.Value{}, fmt.Errorf("transformer: evaluated root term requires callback or allocation semantics")
	}
	if err := e.ctx.Err(); err != nil {
		return product.Value{}, err
	}
	e.values[term], e.valueDone[term] = out, true
	return out, nil
}

func (e *evaluatedTermEvaluator) guard(guard Guard) (bool, error) {
	if e.guardDone[guard] {
		return e.guards[guard], nil
	}
	if err := e.checkpoint(); err != nil {
		return false, err
	}
	if guard == 0 || int(guard) >= len(e.arena.guards) {
		return false, fmt.Errorf("transformer: invalid evaluated guard %d", guard)
	}
	if e.guardVisiting[guard] {
		return false, fmt.Errorf("transformer: cyclic evaluated guard %d", guard)
	}
	e.guardVisiting[guard] = true
	defer delete(e.guardVisiting, guard)

	node := e.arena.guards[guard]
	var holds bool
	switch node.op {
	case guardTrue:
		holds = true
	case guardFalse:
		holds = false
	case guardTruthy, guardFalsy:
		value, err := e.value(node.value)
		if err != nil {
			return false, err
		}
		if node.op == guardTruthy {
			holds = valuerefine.CanBeTruthy(e.arena.reg, value)
		} else {
			holds = valuerefine.CanBeFalsy(e.arena.reg, value)
		}
	case guardAnd:
		holds = true
		for _, argument := range node.args {
			value, err := e.guard(argument)
			if err != nil {
				return false, err
			}
			if !value {
				holds = false
				break
			}
		}
	case guardOr:
		for _, argument := range node.args {
			value, err := e.guard(argument)
			if err != nil {
				return false, err
			}
			if value {
				holds = true
				break
			}
		}
	default:
		return false, fmt.Errorf("transformer: unsupported evaluated guard")
	}
	if err := e.ctx.Err(); err != nil {
		return false, err
	}
	e.guards[guard], e.guardDone[guard] = holds, true
	return holds, nil
}
