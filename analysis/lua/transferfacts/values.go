package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/compiler/ast"
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

func (l *lowerer) valueSource(source sourceprovenance.ASTSource) factflow.ValueSource {
	exprRef, hasExpr := l.exprRef(source.Expr)
	return factflow.ValueSource{
		Kind:         source.Kind,
		ExprRef:      exprRef,
		HasExpr:      hasExpr,
		ExprIndex:    source.ExprIndex,
		TargetIndex:  source.TargetIndex,
		ResultIndex:  source.ResultIndex,
		CallPoint:    source.CallPoint,
		HasCallPoint: source.HasCallPoint,
		Final:        source.Final,
		Expanded:     source.Expanded,
		Adjusted:     source.Adjusted,
		OpenTail:     source.OpenTail,
	}
}

func (l *lowerer) argumentValueSources(args []ast.Expr) []factflow.ValueSource {
	if len(args) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, len(args))
	for i, arg := range args {
		out[i] = l.argumentValueSource(arg, i, i == len(args)-1)
	}
	return out
}

func (l *lowerer) argumentValueSource(arg ast.Expr, index int, final bool) factflow.ValueSource {
	exprRef, hasExpr := l.exprRef(arg)
	producer := valueexpr.TopLevelProducer(arg)
	kind := producerValueSourceKind(producer.Kind)
	expanded := final && valueexpr.CanProduceMultipleValues(arg) && !valueexpr.AdjustRet(arg)
	source := factflow.ValueSource{
		Kind:        kind,
		ExprRef:     exprRef,
		HasExpr:     hasExpr,
		ExprIndex:   index,
		TargetIndex: index,
		ResultIndex: 0,
		Final:       final,
		Expanded:    expanded,
		Adjusted:    valueexpr.CanProduceMultipleValues(arg) && !expanded,
	}
	if producer.Kind == valueexpr.ProducerCall && producer.Call != nil {
		if point, ok := l.callPoints[producer.Call]; ok {
			source.CallPoint = point
			source.HasCallPoint = point != 0
		}
	}
	return source
}

func (l *lowerer) argumentSemanticValueSource(arg ast.Expr, index int, final bool) sourceprovenance.ASTSource {
	producer := valueexpr.TopLevelProducer(arg)
	expanded := final && valueexpr.CanProduceMultipleValues(arg) && !valueexpr.AdjustRet(arg)
	source := sourceprovenance.ASTSource{
		Kind:        producerValueSourceKind(producer.Kind),
		Expr:        arg,
		ExprIndex:   index,
		TargetIndex: index,
		ResultIndex: 0,
		Final:       final,
		Expanded:    expanded,
		Adjusted:    valueexpr.CanProduceMultipleValues(arg) && !expanded,
	}
	if producer.Kind == valueexpr.ProducerCall && producer.Call != nil {
		if point, ok := l.callPoints[producer.Call]; ok {
			source.CallPoint = point
			source.HasCallPoint = point != 0
		}
	}
	return source
}

func producerValueSourceKind(kind valueexpr.ProducerKind) factflow.ValueSourceKind {
	switch kind {
	case valueexpr.ProducerCall:
		return factflow.ValueSourceCall
	case valueexpr.ProducerVararg:
		return factflow.ValueSourceVararg
	default:
		return factflow.ValueSourceExpression
	}
}

func (l *lowerer) exprRef(expr any) (factflow.ExprRef, bool) {
	if expr == nil {
		return 0, false
	}
	if ref, ok := l.exprs[expr]; ok {
		return ref, true
	}
	ref := factflow.ExprRef(len(l.exprs) + 1)
	l.exprs[expr] = ref
	return ref, true
}

func (l *lowerer) typeRefs(types []ast.TypeExpr) []factflow.TypeRef {
	if len(types) == 0 {
		return nil
	}
	out := make([]factflow.TypeRef, len(types))
	for i := range types {
		out[i], _ = l.typeRef(types[i])
	}
	return out
}

func (l *lowerer) typeRef(typ any) (factflow.TypeRef, bool) {
	if typ == nil {
		return 0, false
	}
	if ref, ok := l.types[typ]; ok {
		return ref, true
	}
	if l.types == nil {
		l.types = make(map[any]factflow.TypeRef)
	}
	ref := factflow.TypeRef(len(l.types) + 1)
	l.types[typ] = ref
	return ref, true
}
