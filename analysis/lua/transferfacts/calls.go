package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) callSiteFromWIR(point cfg.Point) (factflow.CallSite, bool) {
	inst, ok := l.wirCallInstruction(point)
	if !ok {
		return factflow.CallSite{}, false
	}
	args, ok := l.callArgumentSourcesFromWIR(point)
	if !ok {
		return factflow.CallSite{}, false
	}
	shape, _ := l.callShapeFromWIR(point)
	metadata, _ := l.callSiteMetadataFromWIR(point)
	receiverSource, hasReceiverSource := l.callReceiverSourceFromWIR(point, valueSourceShape{
		exprIndex:   0,
		targetIndex: 0,
		final:       true,
	})
	if !hasReceiverSource && inst.Call.Method != 0 && inst.Call.Receiver.Kind != wir.OperandNone {
		receiverSource = factflow.NewUnknownValueSource(0)
		hasReceiverSource = true
	}
	exprRef, hasExpr := l.wirCallExprRef(inst)
	return factflow.NewCallSite(factflow.CallSiteConfig{
		Context:            wirCallSiteContext(inst.CallContext),
		Point:              point,
		HasPoint:           true,
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
		ExprIndex:          inst.CallExpr,
		ConditionNegated:   inst.CallConditionNegated,
		ArgumentSources:    args,
		CallSpan:           metadata.callSpan,
		CalleeSpan:         metadata.calleeSpan,
		ArgumentSpans:      metadata.argumentSpans,
		ArgumentLabels:     metadata.argumentLabels,
		TypeArgs:           l.typeRefsFromWIR(inst.CallTypeArgs),
		ResultTargets:      lowerWIRCallResultTargets(l.wir.CallResultTargets(point)),
		Final:              inst.CallFinal,
		Expanded:           inst.CallExpanded,
		Adjusted:           inst.CallAdjusted,
		OpenTail:           inst.CallOpenTail,
	}), true
}

type wirTypeRefKey struct {
	ref wir.TypeRef
}

func (l *lowerer) typeRefsFromWIR(r wir.TypeRefRange) []factflow.TypeRef {
	if l == nil || l.wir == nil || r.Len == 0 {
		return nil
	}
	refs := l.wir.TypeRefs(r)
	if len(refs) == 0 {
		return nil
	}
	out := make([]factflow.TypeRef, 0, len(refs))
	for _, ref := range refs {
		if ref == 0 {
			continue
		}
		if outRef, ok := l.typeRef(wirTypeRefKey{ref: ref}); ok {
			out = append(out, outRef)
		}
	}
	return out
}

func (l *lowerer) wirCallExprRef(inst wir.Instruction) (factflow.ExprRef, bool) {
	if inst.ExprID == 0 {
		return 0, false
	}
	return l.exprRef(wirCallExprRefKey{id: inst.ExprID})
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

type callSiteMetadata struct {
	callSpan       factflow.SourceSpan
	calleeSpan     factflow.SourceSpan
	argumentSpans  []factflow.SourceSpan
	argumentLabels []string
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
	if source, ok := l.valueSourceFromWIROperand(
		inst.Call.Receiver,
		shape.exprIndex,
		shape.targetIndex,
		shape.final,
		shape.expanded,
		shape.openTail,
	); ok {
		return source, true
	}
	if inst.Call.Receiver.Kind != wir.OperandNone {
		return factflow.NewUnknownValueSource(shape.exprIndex), true
	}
	return factflow.ValueSource{}, false
}

func (l *lowerer) callArgumentSourcesFromWIR(point cfg.Point) ([]factflow.ValueSource, bool) {
	if l == nil || l.wir == nil {
		return nil, false
	}
	inst, ok := l.wirCallInstruction(point)
	if !ok {
		return nil, false
	}
	ops := l.wir.Operands(inst.List)
	if len(ops) == 0 {
		return nil, true
	}
	out := make([]factflow.ValueSource, len(ops))
	for i, op := range ops {
		final := i == len(ops)-1
		source, ok := l.callArgumentSourceFromWIROperand(point, op, i, i, final, inst.ListSpread && final)
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
	return l.valueSourceFromWIROperand(op, exprIndex, targetIndex, final, expanded, false)
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
