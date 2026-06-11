// Package transferfacts lowers Lua semantic sidecars into generic transfer facts.
package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Lower converts Lua semantic facts into the generic transfer fact DTOs consumed
// by the engine. It intentionally lowers only syntax facts already represented
// by transfer.Facts; higher semantic layers add branch, iterator, interproc,
// and diagnostic facts separately.
func Lower(result *semantics.Result, graph cfg.Graph) transfer.Facts {
	if result == nil || graph == nil {
		return transfer.NewFacts(transfer.FactsInput{})
	}
	l := lowerer{
		exprs:      make(map[any]transfer.ExprRef),
		types:      make(map[any]transfer.TypeRef),
		callPoints: callPointsByExpr(result, graph),
	}
	input := transfer.FactsInput{
		LocalAssignments:    make(map[cfg.Point]transfer.RootAssignment),
		OrdinaryAssignments: make(map[cfg.Point]transfer.RootAssignment),
		PathAssignments:     make(map[cfg.Point]transfer.PathAssignment),
		BranchRefinements:   make(map[cfg.Point]transfer.BranchRefinement),
		Returns:             make(map[cfg.Point]transfer.Return),
		Calls:               make(map[cfg.Point]transfer.CallProducer),
		CallSites:           make(map[cfg.Point]transfer.CallSite),
		ObjectLiterals:      make(map[transfer.ExprRef]transfer.ObjectLiteral),
		ValueOverlays:       make(map[transfer.ExprRef]transfer.ValueOverlay),
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			if lowered, ok := l.localAssignment(fact); ok {
				input.LocalAssignments[point] = lowered
				l.addAssertionOverlaysForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			if lowered, ok := l.pathAssignment(fact); ok {
				input.PathAssignments[point] = lowered
				l.addAssertionOverlaysForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
			} else if lowered, ok := l.ordinaryAssignment(fact); ok {
				input.OrdinaryAssignments[point] = lowered
				l.addAssertionOverlaysForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
			}
		}
		if fact, ok := result.Return(point); ok {
			input.Returns[point] = transfer.NewReturn(l.valueSources(fact.Sources))
			for _, source := range fact.Sources {
				l.addAssertionOverlaysForSource(&input, source)
			}
		}
		if fact, ok := result.Call(point); ok {
			input.CallSites[point] = l.callSite(fact)
			for i, arg := range fact.Args {
				source := l.argumentSemanticValueSource(arg, i, i == len(fact.Args)-1)
				l.addAssertionOverlaysForSource(&input, source)
				l.addObjectLiteral(&input, result, source)
			}
			if lowered, ok := l.callProducer(fact); ok {
				input.Calls[point] = lowered
			}
		}
		if fact, ok := result.BranchCondition(point); ok {
			if lowered, ok := l.branchRefinement(fact); ok {
				input.BranchRefinements[point] = lowered
			}
			l.addAssertionOverlaysForSource(&input, fact.Source)
		}
	}
	return transfer.NewFacts(input)
}

type lowerer struct {
	exprs      map[any]transfer.ExprRef
	types      map[any]transfer.TypeRef
	callPoints map[*ast.FuncCallExpr]cfg.Point
}

func callPointsByExpr(result *semantics.Result, graph cfg.Graph) map[*ast.FuncCallExpr]cfg.Point {
	out := make(map[*ast.FuncCallExpr]cfg.Point)
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		out[fact.Call] = point
	}
	return out
}

func (l *lowerer) localAssignment(fact semantics.LocalAssignmentFact) (transfer.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return transfer.RootAssignment{}, false
	}
	target := path.NewPath(fact.Symbol, fact.Name)
	return transfer.NewRootAssignment(fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) ordinaryAssignment(fact semantics.OrdinaryAssignmentFact) (transfer.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return transfer.RootAssignment{}, false
	}
	target := fact.Path
	if !fact.HasPath {
		target = path.NewPath(fact.Symbol, "")
	}
	if len(target.Segments) != 0 {
		return transfer.RootAssignment{}, false
	}
	return transfer.NewRootAssignment(fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) pathAssignment(fact semantics.OrdinaryAssignmentFact) (transfer.PathAssignment, bool) {
	if !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
		return transfer.PathAssignment{}, false
	}
	return transfer.NewPathAssignment(fact.Path, l.valueSource(fact.Source)), true
}

func (l *lowerer) callProducer(fact semantics.CallFact) (transfer.CallProducer, bool) {
	context, ok := callProducerContext(fact.Context)
	if !ok {
		return transfer.CallProducer{}, false
	}
	exprRef, hasExpr := l.exprRef(fact.Call)
	calleeSymbol := symbol.ID(0)
	if fact.HasCalleeSymbol {
		calleeSymbol = fact.CalleeSymbol
	}
	calleePath := path.Path{}
	if fact.HasCalleePath {
		calleePath = fact.CalleePath
	}
	return transfer.NewCallProducer(transfer.CallProducerConfig{
		Context:       context,
		CalleeSymbol:  calleeSymbol,
		CalleePath:    calleePath,
		ExprRef:       exprRef,
		HasExpr:       hasExpr,
		ExprIndex:     fact.ExprIndex,
		ResultTargets: l.callProducerResultTargets(fact.ResultTargets),
		Final:         fact.Final,
		Expanded:      fact.Expanded,
		Adjusted:      fact.Adjusted,
		OpenTail:      fact.OpenTail,
	}), true
}

func (l *lowerer) callSite(fact semantics.CallFact) transfer.CallSite {
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
	return transfer.NewCallSite(transfer.CallSiteConfig{
		Context:         callSiteContext(fact.Context),
		CalleeSymbol:    calleeSymbol,
		CalleePath:      calleePath,
		ReceiverPath:    receiverPath,
		HasReceiverPath: fact.HasReceiverPath,
		MethodPath:      methodPath,
		HasMethodPath:   fact.HasMethodPath,
		MethodName:      fact.Method,
		ExprRef:         exprRef,
		HasExpr:         hasExpr,
		ExprIndex:       fact.ExprIndex,
		ArgumentSources: l.argumentValueSources(fact.Args),
		TypeArgs:        l.typeRefs(fact.TypeArgs),
		ResultTargets:   l.callSiteResultTargets(fact.ResultTargets),
		Final:           fact.Final,
		Expanded:        fact.Expanded,
		Adjusted:        fact.Adjusted,
		OpenTail:        fact.OpenTail,
	})
}

func (l *lowerer) addObjectLiteral(input *transfer.FactsInput, result *semantics.Result, source semantics.ValueSource) {
	fact, ok := result.ObjectLiteral(source.Expr)
	if !ok {
		return
	}
	exprRef, hasExpr := l.exprRef(fact.Expr)
	if !hasExpr {
		return
	}
	lowered := l.objectLiteral(fact)
	if len(lowered.Entries()) == 0 {
		return
	}
	if input.ObjectLiterals == nil {
		input.ObjectLiterals = make(map[transfer.ExprRef]transfer.ObjectLiteral)
	}
	input.ObjectLiterals[exprRef] = lowered
	for _, entry := range fact.Entries {
		l.addAssertionOverlaysForSource(input, entry.Source)
	}
}

func (l *lowerer) objectLiteral(fact semantics.ObjectLiteralFact) transfer.ObjectLiteral {
	entries := make([]transfer.ObjectEntry, 0, len(fact.Entries))
	for _, entry := range fact.Entries {
		entries = append(entries, transfer.NewObjectEntry(entry.Suffix, l.valueSource(entry.Source)))
	}
	return transfer.NewObjectLiteral(entries)
}

func (l *lowerer) branchRefinement(fact semantics.BranchConditionFact) (transfer.BranchRefinement, bool) {
	target := fact.Check.Path
	if target.IsEmpty() {
		return transfer.BranchRefinement{}, false
	}
	switch fact.Check.Kind {
	case branchcond.CheckNil:
		return transfer.NewBranchRefinement(
			target,
			presenceRefinement(presence.Absent()), true,
			presenceRefinement(presence.Present()), true,
		), true
	case branchcond.CheckNotNil:
		return transfer.NewBranchRefinement(
			target,
			presenceRefinement(presence.Present()), true,
			presenceRefinement(presence.Absent()), true,
		), true
	case branchcond.CheckTruthy:
		return transfer.NewBranchRefinement(
			target,
			presenceRefinement(presence.Present()), true,
			transfer.ValueRefinement{}, false,
		), true
	case branchcond.CheckFalsy:
		return transfer.NewBranchRefinement(
			target,
			transfer.ValueRefinement{}, false,
			presenceRefinement(presence.Present()), true,
		), true
	case branchcond.CheckTypeEqual, branchcond.CheckTypeNot:
		return l.typeBranchRefinement(target, fact.Check.Kind, fact.Check.TypeName)
	default:
		return transfer.BranchRefinement{}, false
	}
}

func (l *lowerer) typeBranchRefinement(target path.Path, kind branchcond.CheckKind, typeName string) (transfer.BranchRefinement, bool) {
	tag, ok := runtimeTag(typeName)
	if !ok {
		return transfer.BranchRefinement{}, false
	}
	matched := typeMatchedRefinement(tag)
	unmatched := typeUnmatchedRefinement(tag)
	if kind == branchcond.CheckTypeNot {
		return transfer.NewBranchRefinement(target, unmatched, true, matched, true), true
	}
	return transfer.NewBranchRefinement(target, matched, true, unmatched, true), true
}

func typeMatchedRefinement(tag runtimekind.Tag) transfer.ValueRefinement {
	value := runtimeKindRefinement(runtimekind.Singleton(tag))
	if tag == runtimekind.Nil {
		return value.WithConstraint(product.DefaultRegistry(), presenceConstraint(presence.Absent()))
	}
	return value.WithConstraint(product.DefaultRegistry(), presenceConstraint(presence.Present()))
}

func typeUnmatchedRefinement(tag runtimekind.Tag) transfer.ValueRefinement {
	value := runtimeKindRefinement(runtimekind.Top().Without(tag))
	if tag == runtimekind.Nil {
		return value.WithConstraint(product.DefaultRegistry(), presenceConstraint(presence.Present()))
	}
	return value
}

func presenceRefinement(value presence.Value) transfer.ValueRefinement {
	return transfer.NewValueConstraint(presenceConstraint(value))
}

func presenceConstraint(value presence.Value) product.Value {
	return product.NewWithPresence(product.DefaultRegistry(), product.ShapeTop, value)
}

func runtimeKindRefinement(value runtimekind.Value) transfer.ValueRefinement {
	return transfer.NewValueConstraint(runtimeKindConstraint(value))
}

func runtimeKindConstraint(value runtimekind.Value) product.Value {
	return product.Set(product.DefaultRegistry(), product.Top(), runtimekind.Key, value)
}

func runtimeTag(typeName string) (runtimekind.Tag, bool) {
	switch typeName {
	case "nil":
		return runtimekind.Nil, true
	case "boolean":
		return runtimekind.Boolean, true
	case "number":
		return runtimekind.Number, true
	case "string":
		return runtimekind.String, true
	case "table":
		return runtimekind.Table, true
	case "function":
		return runtimekind.Function, true
	case "thread":
		return runtimekind.Thread, true
	case "userdata":
		return runtimekind.Userdata, true
	default:
		return 0, false
	}
}

func (l *lowerer) valueSources(sources []semantics.ValueSource) []transfer.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]transfer.ValueSource, len(sources))
	for i := range sources {
		out[i] = l.valueSource(sources[i])
	}
	return out
}

func (l *lowerer) valueSource(source semantics.ValueSource) transfer.ValueSource {
	exprRef, hasExpr := l.exprRef(source.Expr)
	return transfer.ValueSource{
		Kind:         valueSourceKind(source.Kind),
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

func (l *lowerer) argumentValueSources(args []ast.Expr) []transfer.ValueSource {
	if len(args) == 0 {
		return nil
	}
	out := make([]transfer.ValueSource, len(args))
	for i, arg := range args {
		out[i] = l.argumentValueSource(arg, i, i == len(args)-1)
	}
	return out
}

func (l *lowerer) argumentValueSource(arg ast.Expr, index int, final bool) transfer.ValueSource {
	exprRef, hasExpr := l.exprRef(arg)
	producer := valueexpr.TopLevelProducer(arg)
	kind := argumentTransferSourceKind(producer.Kind)
	expanded := final && valueexpr.CanProduceMultipleValues(arg) && !valueexpr.AdjustRet(arg)
	source := transfer.ValueSource{
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

func (l *lowerer) argumentSemanticValueSource(arg ast.Expr, index int, final bool) semantics.ValueSource {
	producer := valueexpr.TopLevelProducer(arg)
	expanded := final && valueexpr.CanProduceMultipleValues(arg) && !valueexpr.AdjustRet(arg)
	source := semantics.ValueSource{
		Kind:        argumentSemanticSourceKind(producer.Kind),
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

func argumentTransferSourceKind(kind valueexpr.ProducerKind) transfer.ValueSourceKind {
	switch kind {
	case valueexpr.ProducerCall:
		return transfer.ValueSourceCall
	case valueexpr.ProducerVararg:
		return transfer.ValueSourceVararg
	default:
		return transfer.ValueSourceExpression
	}
}

func argumentSemanticSourceKind(kind valueexpr.ProducerKind) semantics.ValueSourceKind {
	switch kind {
	case valueexpr.ProducerCall:
		return semantics.ValueSourceCall
	case valueexpr.ProducerVararg:
		return semantics.ValueSourceVararg
	default:
		return semantics.ValueSourceExpression
	}
}

func (l *lowerer) addAssertionOverlaysForSource(input *transfer.FactsInput, source semantics.ValueSource) {
	if input == nil || source.Expr == nil {
		return
	}
	switch expr := source.Expr.(type) {
	case *ast.CastExpr:
		l.addAssertion(input, source, expr.Expr, castAssertionValue(expr.Type))
	case *ast.NonNilAssertExpr:
		l.addAssertion(input, source, expr.Expr, assertion.NonNil())
	}
}

func (l *lowerer) addAssertion(input *transfer.FactsInput, outer semantics.ValueSource, innerExpr ast.Expr, value assertion.Value) {
	outerRef, hasOuter := l.exprRef(outer.Expr)
	if !hasOuter || innerExpr == nil {
		return
	}
	inner := outer
	inner.Expr = innerExpr
	if input.ValueOverlays == nil {
		input.ValueOverlays = make(map[transfer.ExprRef]transfer.ValueOverlay)
	}
	input.ValueOverlays[outerRef] = transfer.NewValueOverlay(l.valueSource(inner), assertionOverlay(value))
	l.addAssertionOverlaysForSource(input, inner)
}

func assertionOverlay(value assertion.Value) product.Value {
	return product.Set(product.DefaultRegistry(), product.Top(), assertion.Key, value)
}

func castAssertionValue(typ ast.TypeExpr) assertion.Value {
	if primitive, ok := typ.(*ast.PrimitiveTypeExpr); ok && primitive.Name == "any" {
		return assertion.Any()
	}
	return assertion.Type()
}

func (l *lowerer) callProducerResultTargets(targets []semantics.CallResultTarget) []transfer.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]transfer.CallResultTarget, 0, len(targets))
	for _, target := range targets {
		if lowered, ok := l.callProducerResultTarget(target); ok {
			out = append(out, lowered)
		}
	}
	return out
}

func (l *lowerer) callProducerResultTarget(target semantics.CallResultTarget) (transfer.CallResultTarget, bool) {
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		if !target.HasSymbol || target.Symbol == 0 {
			return transfer.CallResultTarget{}, false
		}
		targetPath := target.Path
		if !target.HasPath {
			targetPath = path.NewPath(target.Symbol, target.Name)
		}
		return transfer.NewCallResultTarget(transfer.CallResultTargetLocalAssignment, target.Index, target.Symbol, targetPath), true
	case semantics.CallResultTargetOrdinaryAssignment:
		if !target.HasSymbol || target.Symbol == 0 {
			return transfer.CallResultTarget{}, false
		}
		if target.HasPath && len(target.Path.Segments) != 0 {
			return transfer.CallResultTarget{}, false
		}
		targetPath := target.Path
		if !target.HasPath {
			targetPath = path.NewPath(target.Symbol, "")
		}
		return transfer.NewCallResultTarget(transfer.CallResultTargetOrdinaryAssignment, target.Index, target.Symbol, targetPath), true
	case semantics.CallResultTargetReturn:
		return transfer.NewCallResultTarget(transfer.CallResultTargetReturn, target.Index, 0, path.Path{}), true
	default:
		return transfer.CallResultTarget{}, false
	}
}

func (l *lowerer) callSiteResultTargets(targets []semantics.CallResultTarget) []transfer.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]transfer.CallResultTarget, len(targets))
	for i := range targets {
		out[i] = callSiteResultTarget(targets[i])
	}
	return out
}

func callSiteResultTarget(target semantics.CallResultTarget) transfer.CallResultTarget {
	targetKind := transfer.CallResultTargetUnknown
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		targetKind = transfer.CallResultTargetLocalAssignment
	case semantics.CallResultTargetOrdinaryAssignment:
		targetKind = transfer.CallResultTargetOrdinaryAssignment
	case semantics.CallResultTargetReturn:
		targetKind = transfer.CallResultTargetReturn
	}
	targetSymbol := symbol.ID(0)
	if target.HasSymbol {
		targetSymbol = target.Symbol
	}
	targetPath := path.Path{}
	if target.HasPath {
		targetPath = target.Path
	} else if target.Kind == semantics.CallResultTargetLocalAssignment && target.HasSymbol {
		targetPath = path.NewPath(target.Symbol, target.Name)
	} else if target.Kind == semantics.CallResultTargetOrdinaryAssignment && target.HasSymbol {
		targetPath = path.NewPath(target.Symbol, "")
	}
	return transfer.NewCallResultTarget(targetKind, target.Index, targetSymbol, targetPath)
}

func (l *lowerer) exprRef(expr any) (transfer.ExprRef, bool) {
	if expr == nil {
		return 0, false
	}
	if ref, ok := l.exprs[expr]; ok {
		return ref, true
	}
	ref := transfer.ExprRef(len(l.exprs) + 1)
	l.exprs[expr] = ref
	return ref, true
}

func (l *lowerer) typeRefs(types []ast.TypeExpr) []transfer.TypeRef {
	if len(types) == 0 {
		return nil
	}
	out := make([]transfer.TypeRef, len(types))
	for i := range types {
		out[i], _ = l.typeRef(types[i])
	}
	return out
}

func (l *lowerer) typeRef(typ any) (transfer.TypeRef, bool) {
	if typ == nil {
		return 0, false
	}
	if ref, ok := l.types[typ]; ok {
		return ref, true
	}
	if l.types == nil {
		l.types = make(map[any]transfer.TypeRef)
	}
	ref := transfer.TypeRef(len(l.types) + 1)
	l.types[typ] = ref
	return ref, true
}

func callProducerContext(kind semantics.CallContextKind) (transfer.CallProducerContext, bool) {
	switch kind {
	case semantics.CallContextAssignmentSource:
		return transfer.CallProducerContextAssignment, true
	case semantics.CallContextReturnSource:
		return transfer.CallProducerContextReturn, true
	default:
		return transfer.CallProducerContextUnknown, false
	}
}

func callSiteContext(kind semantics.CallContextKind) transfer.CallSiteContext {
	switch kind {
	case semantics.CallContextStatement:
		return transfer.CallSiteContextStatement
	case semantics.CallContextAssignmentSource:
		return transfer.CallSiteContextAssignmentSource
	case semantics.CallContextReturnSource:
		return transfer.CallSiteContextReturnSource
	case semantics.CallContextIteratorSource:
		return transfer.CallSiteContextIteratorSource
	case semantics.CallContextCondition:
		return transfer.CallSiteContextCondition
	default:
		return transfer.CallSiteContextUnknown
	}
}

func valueSourceKind(kind semantics.ValueSourceKind) transfer.ValueSourceKind {
	switch kind {
	case semantics.ValueSourceExpression:
		return transfer.ValueSourceExpression
	case semantics.ValueSourceCall:
		return transfer.ValueSourceCall
	case semantics.ValueSourceVararg:
		return transfer.ValueSourceVararg
	case semantics.ValueSourceNil:
		return transfer.ValueSourceNil
	default:
		return transfer.ValueSourceUnknown
	}
}
