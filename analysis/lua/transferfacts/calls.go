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
	return l.callSiteWithArgumentSources(fact, args), true
}

func (l *lowerer) callSite(fact semantics.CallFact) factflow.CallSite {
	return l.callSiteWithArgumentSources(fact, l.valueSources(fact.ArgumentSources))
}

func (l *lowerer) callSiteWithArgumentSources(fact semantics.CallFact, argumentSources []factflow.ValueSource) factflow.CallSite {
	exprRef, hasExpr := l.exprRef(fact.Call)
	calleeSymbol := symbol.ID(0)
	if fact.HasCalleeSymbol {
		calleeSymbol = fact.CalleeSymbol
	}
	calleePath := path.Path{}
	if fact.HasCalleePath {
		calleePath = fact.CalleePath
	}
	receiverPath := path.Path{}
	if fact.HasReceiverPath {
		receiverPath = fact.ReceiverPath
	}
	methodPath := path.Path{}
	if fact.HasMethodPath {
		methodPath = fact.MethodPath
	}
	receiverSource := factflow.ValueSource{}
	if fact.HasReceiverSource {
		receiverSource = l.valueSource(fact.ReceiverSource)
	}
	return factflow.NewCallSite(factflow.CallSiteConfig{
		Context:            callSiteContext(fact.Context),
		CalleeSymbol:       calleeSymbol,
		CalleePath:         calleePath,
		CalleeMemberAccess: fact.CalleeMemberAccess,
		ReceiverPath:       receiverPath,
		HasReceiverPath:    fact.HasReceiverPath,
		MethodPath:         methodPath,
		HasMethodPath:      fact.HasMethodPath,
		MethodName:         fact.Method,
		ReceiverSource:     receiverSource,
		HasReceiverSource:  fact.HasReceiverSource,
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
	out := l.valueSources(fallback)
	callResults := l.callResultValueSourcesByTempFromWIR()
	for i, op := range ops {
		final := i == len(ops)-1
		source, ok := l.callArgumentSourceFromWIROperand(point, op, i, i, final, inst.ListSpread && final, callResults)
		if !ok {
			continue
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
