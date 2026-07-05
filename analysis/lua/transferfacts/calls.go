package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) callSiteAt(point cfg.Point, fact semantics.CallFact) (factflow.CallSite, bool) {
	args, ok := l.callArgumentSources(point, fact.ArgumentSources)
	if !ok {
		return factflow.CallSite{}, false
	}
	return l.callSiteWithArgumentSourcesAt(point, fact, args), true
}

func (l *lowerer) callSite(fact semantics.CallFact) factflow.CallSite {
	return l.callSiteWithArgumentSources(fact, l.valueSources(fact.ArgumentSources))
}

func (l *lowerer) callSiteWithArgumentSources(fact semantics.CallFact, argumentSources []factflow.ValueSource) factflow.CallSite {
	shape := semanticCallSiteShape(fact)
	receiverSource, hasReceiverSource := l.semanticReceiverSource(fact)
	return l.callSiteWithArgumentSourcesWithShape(fact, argumentSources, shape, receiverSource, hasReceiverSource)
}

func (l *lowerer) callSiteWithArgumentSourcesAt(point cfg.Point, fact semantics.CallFact, argumentSources []factflow.ValueSource) factflow.CallSite {
	shape := semanticCallSiteShape(fact)
	if wirShape, ok := l.methodCallShapeFromWIR(point); ok {
		shape = wirShape
	} else if wirShape, ok := l.directCallShapeFromWIR(point); ok {
		shape = wirShape
	}
	receiverSource, hasReceiverSource := l.callReceiverSource(point, fact)
	return l.callSiteWithArgumentSourcesWithShape(fact, argumentSources, shape, receiverSource, hasReceiverSource)
}

type callSiteShape struct {
	calleeSymbol       symbol.ID
	calleePath         path.Path
	calleeMemberAccess bool
	receiverPath       path.Path
	hasReceiverPath    bool
	methodPath         path.Path
	hasMethodPath      bool
	methodName         string
}

type valueSourceShape struct {
	exprIndex   int
	targetIndex int
	final       bool
	expanded    bool
	openTail    bool
}

func semanticCallSiteShape(fact semantics.CallFact) callSiteShape {
	shape := callSiteShape{}
	if fact.HasCalleeSymbol {
		shape.calleeSymbol = fact.CalleeSymbol
	}
	if fact.HasCalleePath {
		shape.calleePath = fact.CalleePath
	}
	shape.calleeMemberAccess = fact.CalleeMemberAccess
	if fact.HasReceiverPath {
		shape.receiverPath = fact.ReceiverPath
		shape.hasReceiverPath = true
	}
	if fact.HasMethodPath {
		shape.methodPath = fact.MethodPath
		shape.hasMethodPath = true
	}
	shape.methodName = fact.Method
	return shape
}

func (l *lowerer) callSiteWithArgumentSourcesWithShape(
	fact semantics.CallFact,
	argumentSources []factflow.ValueSource,
	shape callSiteShape,
	receiverSource factflow.ValueSource,
	hasReceiverSource bool,
) factflow.CallSite {
	exprRef, hasExpr := l.exprRef(fact.Call)
	return factflow.NewCallSite(factflow.CallSiteConfig{
		Context:            callSiteContext(fact.Context),
		CalleeSymbol:       shape.calleeSymbol,
		CalleePath:         shape.calleePath,
		CalleeMemberAccess: shape.calleeMemberAccess,
		ReceiverPath:       shape.receiverPath,
		HasReceiverPath:    shape.hasReceiverPath,
		MethodPath:         shape.methodPath,
		HasMethodPath:      shape.hasMethodPath,
		MethodName:         shape.methodName,
		ReceiverSource:     receiverSource,
		HasReceiverSource:  hasReceiverSource,
		ExprRef:            exprRef,
		HasExpr:            hasExpr,
		ExprIndex:          fact.ExprIndex,
		ConditionNegated:   fact.ConditionNegated,
		ArgumentSources:    argumentSources,
		CallSpan:           sourceSpan(fact.CallSpan),
		CalleeSpan:         sourceSpan(fact.CalleeSpan),
		ArgumentSpans:      sourceSpans(fact.ArgumentSpans),
		ArgumentLabels:     append([]string(nil), fact.ArgumentLabels...),
		TypeArgs:           l.typeRefs(fact.TypeArgs),
		ResultTargets:      l.evidenceCallSiteResultTargets(fact.ResultTargets),
		Final:              fact.Final,
		Expanded:           fact.Expanded,
		Adjusted:           fact.Adjusted,
		OpenTail:           fact.OpenTail,
	})
}

func (l *lowerer) methodCallShapeFromWIR(point cfg.Point) (callSiteShape, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok || inst.Call.Method == 0 || inst.Call.Receiver.Kind != wir.OperandPath {
		return callSiteShape{}, false
	}
	receiverPath := l.wir.Path(wir.PathRef(inst.Call.Receiver.Ref))
	if receiverPath.Symbol == 0 {
		return callSiteShape{}, false
	}
	method := l.wir.Const(inst.Call.Method)
	if method.Kind != wir.ConstString || method.Str == "" {
		return callSiteShape{}, false
	}
	methodPath := receiverPath.Field(method.Str)
	return callSiteShape{
		calleePath:         methodPath,
		calleeMemberAccess: true,
		receiverPath:       receiverPath,
		hasReceiverPath:    true,
		methodPath:         methodPath,
		hasMethodPath:      true,
		methodName:         method.Str,
	}, true
}

func (l *lowerer) directCallShapeFromWIR(point cfg.Point) (callSiteShape, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok || inst.Call.Method != 0 || inst.Call.Callee.Kind != wir.OperandPath {
		return callSiteShape{}, false
	}
	calleePath := l.wir.Path(wir.PathRef(inst.Call.Callee.Ref))
	if calleePath.Symbol == 0 {
		return callSiteShape{}, false
	}
	return callSiteShape{
		calleeSymbol:       calleePath.Symbol,
		calleePath:         calleePath,
		calleeMemberAccess: len(calleePath.Segments) > 0,
	}, true
}

func (l *lowerer) semanticReceiverSource(fact semantics.CallFact) (factflow.ValueSource, bool) {
	if !fact.HasReceiverSource {
		return factflow.ValueSource{}, false
	}
	return l.valueSource(fact.ReceiverSource), true
}

func (l *lowerer) callReceiverSource(point cfg.Point, fact semantics.CallFact) (factflow.ValueSource, bool) {
	if source, ok := l.callReceiverSourceFromWIR(point, valueSourceShape{
		exprIndex:   0,
		targetIndex: 0,
		final:       true,
	}); ok {
		return source, true
	}
	fallback, hasFallback := l.semanticReceiverSource(fact)
	if !hasFallback {
		return factflow.ValueSource{}, false
	}
	return fallback, true
}

func (l *lowerer) callReceiverSourceFromWIR(point cfg.Point, shape valueSourceShape) (factflow.ValueSource, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok || inst.Call.Method == 0 || inst.Call.Receiver.Kind == wir.OperandNone {
		return factflow.ValueSource{}, false
	}
	if source, ok := l.valueSourceFromWIRRootPathOperand(
		inst.Call.Receiver,
		shape.exprIndex,
		shape.targetIndex,
		shape.final,
		symbol.Local,
		symbol.Param,
	); ok {
		return source, true
	}
	return l.valueSourceFromWIROperand(
		inst.Call.Receiver,
		shape.exprIndex,
		shape.targetIndex,
		shape.final,
		shape.expanded,
		shape.openTail,
		l.callResultValueSourcesByTempFromWIR(),
	)
}

func (l *lowerer) callArgumentSources(point cfg.Point, fallback []sourceprovenance.ASTSource) ([]factflow.ValueSource, bool) {
	if l == nil || l.wir == nil {
		return l.valueSources(fallback), true
	}
	inst, ok := l.wirCallInstruction(point)
	if !ok {
		return nil, false
	}
	ops := l.wir.Operands(inst.List)
	if len(ops) != len(fallback) {
		return nil, false
	}
	out := make([]factflow.ValueSource, len(fallback))
	callResults := l.callResultValueSourcesByTempFromWIR()
	for i, op := range ops {
		final := i == len(ops)-1
		source, ok := l.callArgumentSourceFromWIROperand(point, op, i, i, final, inst.ListSpread && final, callResults)
		if !ok {
			source = l.valueSource(fallback[i])
		}
		out[i] = source
	}
	return out, true
}

func (l *lowerer) callArgumentSourceFromWIROperand(
	point cfg.Point,
	op wir.Operand,
	exprIndex int,
	targetIndex int,
	final bool,
	expanded bool,
	callResults map[uint32]wirCallResultSource,
) (factflow.ValueSource, bool) {
	if source, ok := l.localRootPathExpressionSourceFromWIR("call-arg", point, op, exprIndex, targetIndex, final, false, false); ok {
		return source, true
	}
	if source, ok := l.valueSourceFromWIRRootPathOperand(op, exprIndex, targetIndex, final, symbol.Param); ok {
		return source, true
	}
	return l.valueSourceFromWIROperand(op, exprIndex, targetIndex, final, expanded, false, callResults)
}

func (l *lowerer) wirCallInstruction(point cfg.Point) (wir.Instruction, bool) {
	if l == nil || l.wir == nil {
		return wir.Instruction{}, false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op == wir.OpCall {
			return inst, true
		}
	}
	return wir.Instruction{}, false
}

func sourceSpans(in []semantics.SourceSpan) []factflow.SourceSpan {
	if len(in) == 0 {
		return nil
	}
	out := make([]factflow.SourceSpan, len(in))
	for i, span := range in {
		out[i] = factflow.SourceSpan{
			StartLine: span.StartLine,
			StartCol:  span.StartCol,
			EndLine:   span.EndLine,
			EndCol:    span.EndCol,
		}
	}
	return out
}

func callSiteContext(kind semantics.CallContextKind) factflow.CallSiteContext {
	switch kind {
	case semantics.CallContextStatement:
		return factflow.CallSiteContextStatement
	case semantics.CallContextAssignmentSource:
		return factflow.CallSiteContextAssignmentSource
	case semantics.CallContextReturnSource:
		return factflow.CallSiteContextReturnSource
	case semantics.CallContextIteratorSource:
		return factflow.CallSiteContextIteratorSource
	case semantics.CallContextCondition:
		return factflow.CallSiteContextCondition
	case semantics.CallContextExpressionProducer:
		return factflow.CallSiteContextExpressionProducer
	default:
		return factflow.CallSiteContextUnknown
	}
}
