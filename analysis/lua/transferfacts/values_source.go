package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
)

func (l *lowerer) valueSources(sources []sourceprovenance.ASTSource) []factflow.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, len(sources))
	for i := range sources {
		out[i] = l.valueSource(sources[i])
	}
	return out
}

func (l *lowerer) returnValueSources(sources []sourceprovenance.ASTSource, result *semantics.Result) []factflow.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, 0, len(sources))
	for _, source := range sources {
		for _, expanded := range l.expandTypeIsOpenTailReturnSource(source, result) {
			out = append(out, l.valueSource(expanded))
		}
	}
	return out
}

func (l *lowerer) returnValueSourcesFromWIR(point cfg.Point) ([]factflow.ValueSource, bool) {
	if l == nil || l.wir == nil {
		return nil, false
	}
	var ret wir.Instruction
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op == wir.OpReturn {
			ret = inst
			break
		}
	}
	if ret.Op != wir.OpReturn {
		return nil, false
	}
	ops := l.wir.Operands(ret.List)
	out := make([]factflow.ValueSource, len(ops))
	callResults := l.callResultValueSourcesByTempFromWIR()
	for i, op := range ops {
		source, ok := l.returnOperandValueSourceFromWIR(op, i, i == len(ops)-1, ret.ListSpread, callResults)
		if !ok {
			return nil, false
		}
		out[i] = source
	}
	return out, true
}

type wirCallResultSource struct {
	point       cfg.Point
	resultIndex int
}

func (l *lowerer) returnOperandValueSourceFromWIR(
	op wir.Operand,
	index int,
	final bool,
	listSpread bool,
	callResults map[uint32]wirCallResultSource,
) (factflow.ValueSource, bool) {
	switch op.Kind {
	case wir.OperandConst:
		c := l.wir.Const(wir.ConstRef(op.Ref))
		if c.Kind == wir.ConstNil {
			return factflow.NewNilValueSource(index), true
		}
	case wir.OperandVararg:
		shape, ok := factflow.NewValueSourceShape(final, listSpread && final, false, listSpread && final)
		if !ok {
			return factflow.ValueSource{}, false
		}
		return mustValueSource(factflow.NewVarargValueSource(0, index, index, 0, shape)), true
	case wir.OperandTemp:
		if source, ok := callResultValueSourceFromWIR(op, index, final, listSpread, callResults); ok {
			return source, true
		}
	}
	return factflow.ValueSource{}, false
}

func (l *lowerer) callResultValueSourcesByTempFromWIR() map[uint32]wirCallResultSource {
	out := make(map[uint32]wirCallResultSource)
	for i := 0; i < l.wir.Len(); i++ {
		inst := l.wir.Instr(i)
		if inst.Op != wir.OpCall {
			continue
		}
		results := l.wir.Operands(inst.Results)
		for resultIndex, result := range results {
			if result.Kind != wir.OperandTemp {
				continue
			}
			out[result.Ref] = wirCallResultSource{point: inst.Point, resultIndex: resultIndex}
		}
	}
	return out
}

func callResultValueSourceFromWIR(
	op wir.Operand,
	index int,
	final bool,
	listSpread bool,
	callResults map[uint32]wirCallResultSource,
) (factflow.ValueSource, bool) {
	if op.Kind != wir.OperandTemp {
		return factflow.ValueSource{}, false
	}
	result, ok := callResults[op.Ref]
	if !ok {
		return factflow.ValueSource{}, false
	}
	expanded := listSpread && final
	shape, ok := factflow.NewValueSourceShape(final, expanded, !expanded, expanded)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return mustValueSource(factflow.NewCallValueSource(0, index, index, result.resultIndex, result.point, shape)), true
}

func (l *lowerer) expandTypeIsOpenTailReturnSource(source sourceprovenance.ASTSource, result *semantics.Result) []sourceprovenance.ASTSource {
	if source.Kind != sourceprovenance.SourceCall || !source.OpenTail || !source.Expanded ||
		!source.HasCallPoint || result == nil {
		return []sourceprovenance.ASTSource{source}
	}
	view, ok := result.CallView(source.CallPoint)
	if !ok {
		return []sourceprovenance.ASTSource{source}
	}
	fact, _ := view.Borrowed()
	if _, _, ok := l.typeIsCall(fact); !ok {
		return []sourceprovenance.ASTSource{source}
	}
	value := source
	value.OpenTail = false
	errorSource := source
	errorSource.TargetIndex = source.TargetIndex + 1
	errorSource.ResultIndex = source.ResultIndex + 1
	errorSource.OpenTail = false
	return []sourceprovenance.ASTSource{value, errorSource}
}

func (l *lowerer) valueSource(source sourceprovenance.ASTSource) factflow.ValueSource {
	exprRef, hasExpr := l.valueSourceExprRef(source)
	if hasExpr {
		l.addExpressionPath(exprRef, source.Expr)
		l.addExpressionCondition(exprRef, source.Expr)
		if source.Kind == sourceprovenance.SourceExpression {
			l.addExpressionValue(exprRef, source.Expr)
		}
	}
	shape, ok := factflow.NewValueSourceShape(source.Final, source.Expanded, source.Adjusted, source.OpenTail)
	if !ok {
		panic("transferfacts: invalid value source shape")
	}
	switch source.Kind {
	case sourceprovenance.SourceExpression:
		return mustValueSource(factflow.NewExpressionValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, shape))
	case sourceprovenance.SourceCall:
		if !source.HasCallPoint {
			return factflow.NewUnknownValueSource(source.TargetIndex)
		}
		return mustValueSource(factflow.NewCallValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, source.CallPoint, shape))
	case sourceprovenance.SourceVararg:
		return mustValueSource(factflow.NewVarargValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, shape))
	case sourceprovenance.SourceNil:
		return factflow.NewNilValueSource(source.TargetIndex)
	case sourceprovenance.SourceUnknown:
		return factflow.NewUnknownValueSource(source.TargetIndex)
	default:
		panic("transferfacts: unknown value source kind")
	}
}

func mustValueSource(source factflow.ValueSource, ok bool) factflow.ValueSource {
	if !ok {
		panic("transferfacts: invalid value source")
	}
	return source
}
