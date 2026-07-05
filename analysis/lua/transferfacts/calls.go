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
	flags := semanticCallSiteFlags(fact)
	metadata := semanticCallSiteMetadata(fact)
	receiverSource, hasReceiverSource := l.semanticReceiverSource(fact)
	return l.callSiteWithArgumentSourcesWithShape(fact, argumentSources, shape, flags, metadata, receiverSource, hasReceiverSource, l.evidenceCallSiteResultTargets(fact.ResultTargets))
}

func (l *lowerer) callSiteWithArgumentSourcesAt(point cfg.Point, fact semantics.CallFact, argumentSources []factflow.ValueSource) factflow.CallSite {
	shape := semanticCallSiteShape(fact)
	if wirShape, ok := l.callShapeFromWIR(point); ok {
		shape = wirShape
	}
	flags := semanticCallSiteFlags(fact)
	if wirFlags, ok := l.callSiteFlagsFromWIR(point); ok {
		flags = wirFlags
	}
	metadata := semanticCallSiteMetadata(fact)
	if wirMetadata, ok := l.callSiteMetadataFromWIR(point); ok {
		metadata = wirMetadata
	}
	receiverSource, hasReceiverSource := l.callReceiverSource(point, fact)
	return l.callSiteWithArgumentSourcesWithShape(fact, argumentSources, shape, flags, metadata, receiverSource, hasReceiverSource, l.callSiteResultTargetsFromWIR(point, fact.ResultTargets))
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

type callSiteFlags struct {
	context  factflow.CallSiteContext
	expr     int
	final    bool
	expanded bool
	adjusted bool
	openTail bool
}

type callSiteMetadata struct {
	callSpan       factflow.SourceSpan
	calleeSpan     factflow.SourceSpan
	argumentSpans  []factflow.SourceSpan
	argumentLabels []string
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

func semanticCallSiteFlags(fact semantics.CallFact) callSiteFlags {
	return callSiteFlags{
		context:  callSiteContext(fact.Context),
		expr:     fact.ExprIndex,
		final:    fact.Final,
		expanded: fact.Expanded,
		adjusted: fact.Adjusted,
		openTail: fact.OpenTail,
	}
}

func semanticCallSiteMetadata(fact semantics.CallFact) callSiteMetadata {
	return callSiteMetadata{
		callSpan:       sourceSpan(fact.CallSpan),
		calleeSpan:     sourceSpan(fact.CalleeSpan),
		argumentSpans:  sourceSpans(fact.ArgumentSpans),
		argumentLabels: append([]string(nil), fact.ArgumentLabels...),
	}
}

func (l *lowerer) callSiteWithArgumentSourcesWithShape(
	fact semantics.CallFact,
	argumentSources []factflow.ValueSource,
	shape callSiteShape,
	flags callSiteFlags,
	metadata callSiteMetadata,
	receiverSource factflow.ValueSource,
	hasReceiverSource bool,
	resultTargets []factflow.CallResultTarget,
) factflow.CallSite {
	exprRef, hasExpr := l.exprRef(fact.Call)
	return factflow.NewCallSite(factflow.CallSiteConfig{
		Context:            flags.context,
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
		ExprIndex:          flags.expr,
		ConditionNegated:   fact.ConditionNegated,
		ArgumentSources:    argumentSources,
		CallSpan:           metadata.callSpan,
		CalleeSpan:         metadata.calleeSpan,
		ArgumentSpans:      metadata.argumentSpans,
		ArgumentLabels:     metadata.argumentLabels,
		TypeArgs:           l.typeRefs(fact.TypeArgs),
		ResultTargets:      resultTargets,
		Final:              flags.final,
		Expanded:           flags.expanded,
		Adjusted:           flags.adjusted,
		OpenTail:           flags.openTail,
	})
}

func (l *lowerer) callSiteMetadataFromWIR(point cfg.Point) (callSiteMetadata, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok {
		return callSiteMetadata{}, false
	}
	argMeta := l.wir.CallArgumentMeta(inst.CallArgs)
	spans := make([]factflow.SourceSpan, len(argMeta))
	labels := make([]string, len(argMeta))
	for i, meta := range argMeta {
		spans[i] = sourceSpanFromWIR(meta.Span)
		labels[i] = meta.Label
	}
	return callSiteMetadata{
		callSpan:       sourceSpanFromWIR(inst.CallSpan),
		calleeSpan:     sourceSpanFromWIR(inst.CalleeSpan),
		argumentSpans:  spans,
		argumentLabels: labels,
	}, true
}

func (l *lowerer) callSiteFlagsFromWIR(point cfg.Point) (callSiteFlags, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok || inst.CallContext == wir.CallContextUnknown {
		return callSiteFlags{}, false
	}
	return callSiteFlags{
		context:  wirCallSiteContext(inst.CallContext),
		expr:     inst.CallExpr,
		final:    inst.CallFinal,
		expanded: inst.CallExpanded,
		adjusted: inst.CallAdjusted,
		openTail: inst.CallOpenTail,
	}, true
}

func (l *lowerer) callShapeFromWIR(point cfg.Point) (callSiteShape, bool) {
	if l == nil || l.wir == nil {
		return callSiteShape{}, false
	}
	inst, hasCall := l.wirCallInstruction(point)
	if shape, ok := l.methodCallShapeFromWIR(point); ok {
		return shape, true
	}
	if hasCall && inst.Call.Method != 0 {
		return callSiteShape{}, false
	}
	if shape, ok := l.directCallShapeFromWIR(point); ok {
		return shape, true
	}
	if hasCall {
		return callSiteShape{}, true
	}
	return callSiteShape{}, false
}

func (l *lowerer) methodCallShapeFromWIR(point cfg.Point) (callSiteShape, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok || inst.Call.Method == 0 {
		return callSiteShape{}, false
	}
	method := l.wir.Const(inst.Call.Method)
	if method.Kind != wir.ConstString || method.Str == "" {
		return callSiteShape{}, false
	}
	shape := callSiteShape{
		calleeMemberAccess: true,
		methodName:         method.Str,
	}
	if inst.Call.Receiver.Kind != wir.OperandPath {
		return shape, true
	}
	receiverPath := l.wir.Path(wir.PathRef(inst.Call.Receiver.Ref))
	if receiverPath.Symbol == 0 {
		return shape, true
	}
	methodPath := receiverPath.Field(method.Str)
	shape.calleePath = methodPath
	shape.receiverPath = receiverPath
	shape.hasReceiverPath = true
	shape.methodPath = methodPath
	shape.hasMethodPath = true
	return shape, true
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
	if l != nil && l.wir != nil {
		if inst, ok := l.wirCallInstruction(point); ok && inst.Call.Method != 0 && inst.Call.Receiver.Kind != wir.OperandNone {
			return factflow.NewUnknownValueSource(0), true
		}
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
	if source, ok := l.pathExpressionSourceFromWIR(
		"call-receiver",
		point,
		inst.Call.Receiver,
		shape.exprIndex,
		shape.targetIndex,
		shape.final,
		shape.expanded,
		shape.openTail,
		symbol.Local,
		symbol.Param,
		symbol.Global,
		symbol.Upvalue,
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
		l.resultValueSourcesByTempFromWIR(),
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
	resultSources := l.resultValueSourcesByTempFromWIR()
	for i, op := range ops {
		final := i == len(ops)-1
		source, ok := l.callArgumentSourceFromWIROperand(point, op, i, i, final, inst.ListSpread && final, resultSources)
		if !ok {
			source = factflow.NewUnknownValueSource(i)
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
	resultSources map[uint32]wirResultSource,
) (factflow.ValueSource, bool) {
	if source, ok := l.localRootPathExpressionSourceFromWIR("call-arg", point, op, exprIndex, targetIndex, final, false, false); ok {
		return source, true
	}
	if source, ok := l.valueSourceFromWIRRootPathOperand(op, exprIndex, targetIndex, final, symbol.Param); ok {
		return source, true
	}
	if source, ok := l.pathExpressionSourceFromWIR("call-arg", point, op, exprIndex, targetIndex, final, false, false, symbol.Local, symbol.Param, symbol.Global, symbol.Upvalue); ok {
		return source, true
	}
	return l.valueSourceFromWIROperand(op, exprIndex, targetIndex, final, expanded, false, resultSources)
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

func (l *lowerer) callArgumentPathFromWIR(point cfg.Point, index int) (path.Path, bool) {
	if index < 0 {
		return path.Path{}, false
	}
	inst, ok := l.wirCallInstruction(point)
	if !ok {
		return path.Path{}, false
	}
	args := l.wir.Operands(inst.List)
	if index >= len(args) || args[index].Kind != wir.OperandPath {
		return path.Path{}, false
	}
	argPath := l.wir.Path(wir.PathRef(args[index].Ref))
	if argPath.IsEmpty() {
		return path.Path{}, false
	}
	return argPath, true
}

func (l *lowerer) callCalleePathFromWIR(point cfg.Point) (path.Path, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok || inst.Call.Callee.Kind != wir.OperandPath {
		return path.Path{}, false
	}
	calleePath := l.wir.Path(wir.PathRef(inst.Call.Callee.Ref))
	if calleePath.IsEmpty() {
		return path.Path{}, false
	}
	return calleePath, true
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

func wirCallSiteContext(kind wir.CallContextKind) factflow.CallSiteContext {
	switch kind {
	case wir.CallContextStatement:
		return factflow.CallSiteContextStatement
	case wir.CallContextAssignmentSource:
		return factflow.CallSiteContextAssignmentSource
	case wir.CallContextReturnSource:
		return factflow.CallSiteContextReturnSource
	case wir.CallContextIteratorSource:
		return factflow.CallSiteContextIteratorSource
	case wir.CallContextCondition:
		return factflow.CallSiteContextCondition
	case wir.CallContextExpressionProducer:
		return factflow.CallSiteContextExpressionProducer
	default:
		return factflow.CallSiteContextUnknown
	}
}
