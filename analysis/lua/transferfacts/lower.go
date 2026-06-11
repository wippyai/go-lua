// Package transferfacts lowers Lua semantic sidecars into generic transfer facts.
package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/lua/valuesource"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Lower converts Lua semantic facts into the generic transfer fact DTOs consumed
// by the engine. It intentionally lowers only syntax facts already represented
// by factflow.Facts; higher semantic layers add branch, iterator, interproc,
// and diagnostic facts separately.
func Lower(result *semantics.Result, graph cfg.Graph) factflow.Facts {
	if result == nil || graph == nil {
		return factflow.NewFacts(factflow.FactsInput{})
	}
	l := lowerer{
		exprs:      make(map[any]factflow.ExprRef),
		types:      make(map[any]factflow.TypeRef),
		callPoints: callPointsByExpr(result, graph),
	}
	input := factflow.FactsInput{
		LocalAssignments:    make(map[cfg.Point]factflow.RootAssignment),
		OrdinaryAssignments: make(map[cfg.Point]factflow.RootAssignment),
		PathAssignments:     make(map[cfg.Point]factflow.PathAssignment),
		BranchRefinements:   make(map[cfg.Point]factflow.BranchRefinement),
		Returns:             make(map[cfg.Point]factflow.Return),
		Calls:               make(map[cfg.Point]factflow.CallProducer),
		CallSites:           make(map[cfg.Point]factflow.CallSite),
		ObjectLiterals:      make(map[factflow.ExprRef]factflow.ObjectLiteral),
		ValueOverlays:       make(map[factflow.ExprRef]factflow.ValueOverlay),
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
			input.Returns[point] = factflow.NewReturn(l.valueSources(fact.Sources))
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
	return factflow.NewFacts(input)
}

type lowerer struct {
	exprs      map[any]factflow.ExprRef
	types      map[any]factflow.TypeRef
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

func (l *lowerer) localAssignment(fact semantics.LocalAssignmentFact) (factflow.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.RootAssignment{}, false
	}
	target := path.NewPath(fact.Symbol, fact.Name)
	return factflow.NewRootAssignment(fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) ordinaryAssignment(fact semantics.OrdinaryAssignmentFact) (factflow.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.RootAssignment{}, false
	}
	target := fact.Path
	if !fact.HasPath {
		target = path.NewPath(fact.Symbol, "")
	}
	if len(target.Segments) != 0 {
		return factflow.RootAssignment{}, false
	}
	return factflow.NewRootAssignment(fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) pathAssignment(fact semantics.OrdinaryAssignmentFact) (factflow.PathAssignment, bool) {
	if !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
		return factflow.PathAssignment{}, false
	}
	return factflow.NewPathAssignment(fact.Path, l.valueSource(fact.Source)), true
}

func (l *lowerer) callProducer(fact semantics.CallFact) (factflow.CallProducer, bool) {
	context, ok := callProducerContext(fact.Context)
	if !ok {
		return factflow.CallProducer{}, false
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
	return factflow.NewCallProducer(factflow.CallProducerConfig{
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

func (l *lowerer) callSite(fact semantics.CallFact) factflow.CallSite {
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
	return factflow.NewCallSite(factflow.CallSiteConfig{
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

func (l *lowerer) addObjectLiteral(input *factflow.FactsInput, result *semantics.Result, source valuesource.Source) {
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
		input.ObjectLiterals = make(map[factflow.ExprRef]factflow.ObjectLiteral)
	}
	input.ObjectLiterals[exprRef] = lowered
	for _, entry := range fact.Entries {
		l.addAssertionOverlaysForSource(input, entry.Source)
	}
}

func (l *lowerer) objectLiteral(fact semantics.ObjectLiteralFact) factflow.ObjectLiteral {
	entries := make([]factflow.ObjectEntry, 0, len(fact.Entries))
	for _, entry := range fact.Entries {
		entries = append(entries, factflow.NewObjectEntry(entry.Suffix, l.valueSource(entry.Source)))
	}
	return factflow.NewObjectLiteral(entries)
}

func (l *lowerer) branchRefinement(fact semantics.BranchConditionFact) (factflow.BranchRefinement, bool) {
	target := fact.Check.Path
	if target.IsEmpty() {
		return factflow.BranchRefinement{}, false
	}
	switch fact.Check.Kind {
	case branchcond.CheckNil:
		return factflow.NewBranchRefinement(
			target,
			presenceRefinement(presence.Absent()), true,
			presenceRefinement(presence.Present()), true,
		), true
	case branchcond.CheckNotNil:
		return factflow.NewBranchRefinement(
			target,
			presenceRefinement(presence.Present()), true,
			presenceRefinement(presence.Absent()), true,
		), true
	case branchcond.CheckTruthy:
		return factflow.NewBranchRefinement(
			target,
			presenceRefinement(presence.Present()), true,
			factflow.ValueRefinement{}, false,
		), true
	case branchcond.CheckFalsy:
		return factflow.NewBranchRefinement(
			target,
			factflow.ValueRefinement{}, false,
			presenceRefinement(presence.Present()), true,
		), true
	case branchcond.CheckTypeEqual, branchcond.CheckTypeNot:
		return l.typeBranchRefinement(target, fact.Check.Kind, fact.Check.TypeName)
	default:
		return factflow.BranchRefinement{}, false
	}
}

func (l *lowerer) typeBranchRefinement(target path.Path, kind branchcond.CheckKind, typeName string) (factflow.BranchRefinement, bool) {
	tag, ok := runtimeTag(typeName)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	matched := typeMatchedRefinement(tag)
	unmatched := typeUnmatchedRefinement(tag)
	if kind == branchcond.CheckTypeNot {
		return factflow.NewBranchRefinement(target, unmatched, true, matched, true), true
	}
	return factflow.NewBranchRefinement(target, matched, true, unmatched, true), true
}

func typeMatchedRefinement(tag runtimekind.Tag) factflow.ValueRefinement {
	value := runtimeKindRefinement(runtimekind.Singleton(tag))
	if tag == runtimekind.Nil {
		return value.WithConstraint(product.DefaultRegistry(), presenceConstraint(presence.Absent()))
	}
	return value.WithConstraint(product.DefaultRegistry(), presenceConstraint(presence.Present()))
}

func typeUnmatchedRefinement(tag runtimekind.Tag) factflow.ValueRefinement {
	value := runtimeKindRefinement(runtimekind.Top().Without(tag))
	if tag == runtimekind.Nil {
		return value.WithConstraint(product.DefaultRegistry(), presenceConstraint(presence.Present()))
	}
	return value
}

func presenceRefinement(value presence.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(presenceConstraint(value))
}

func presenceConstraint(value presence.Value) product.Value {
	return product.NewWithPresence(product.DefaultRegistry(), product.ShapeTop, value)
}

func runtimeKindRefinement(value runtimekind.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(runtimeKindConstraint(value))
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

func (l *lowerer) valueSources(sources []valuesource.Source) []factflow.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, len(sources))
	for i := range sources {
		out[i] = l.valueSource(sources[i])
	}
	return out
}

func (l *lowerer) valueSource(source valuesource.Source) factflow.ValueSource {
	exprRef, hasExpr := l.exprRef(source.Expr)
	return factflow.ValueSource{
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
	kind := argumentTransferSourceKind(producer.Kind)
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

func (l *lowerer) argumentSemanticValueSource(arg ast.Expr, index int, final bool) valuesource.Source {
	producer := valueexpr.TopLevelProducer(arg)
	expanded := final && valueexpr.CanProduceMultipleValues(arg) && !valueexpr.AdjustRet(arg)
	source := valuesource.Source{
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

func argumentTransferSourceKind(kind valueexpr.ProducerKind) factflow.ValueSourceKind {
	switch kind {
	case valueexpr.ProducerCall:
		return factflow.ValueSourceCall
	case valueexpr.ProducerVararg:
		return factflow.ValueSourceVararg
	default:
		return factflow.ValueSourceExpression
	}
}

func argumentSemanticSourceKind(kind valueexpr.ProducerKind) valuesource.Kind {
	switch kind {
	case valueexpr.ProducerCall:
		return valuesource.Call
	case valueexpr.ProducerVararg:
		return valuesource.Vararg
	default:
		return valuesource.Expression
	}
}

func (l *lowerer) addAssertionOverlaysForSource(input *factflow.FactsInput, source valuesource.Source) {
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

func (l *lowerer) addAssertion(input *factflow.FactsInput, outer valuesource.Source, innerExpr ast.Expr, value assertion.Value) {
	outerRef, hasOuter := l.exprRef(outer.Expr)
	if !hasOuter || innerExpr == nil {
		return
	}
	inner := outer
	inner.Expr = innerExpr
	if input.ValueOverlays == nil {
		input.ValueOverlays = make(map[factflow.ExprRef]factflow.ValueOverlay)
	}
	input.ValueOverlays[outerRef] = factflow.NewValueOverlay(l.valueSource(inner), assertionOverlay(value))
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

func (l *lowerer) callProducerResultTargets(targets []semantics.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, 0, len(targets))
	for _, target := range targets {
		if lowered, ok := l.callProducerResultTarget(target); ok {
			out = append(out, lowered)
		}
	}
	return out
}

func (l *lowerer) callProducerResultTarget(target semantics.CallResultTarget) (factflow.CallResultTarget, bool) {
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		if !target.HasSymbol || target.Symbol == 0 {
			return factflow.CallResultTarget{}, false
		}
		targetPath := target.Path
		if !target.HasPath {
			targetPath = path.NewPath(target.Symbol, target.Name)
		}
		return factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, target.Index, target.Symbol, targetPath), true
	case semantics.CallResultTargetOrdinaryAssignment:
		if !target.HasSymbol || target.Symbol == 0 {
			return factflow.CallResultTarget{}, false
		}
		if target.HasPath && len(target.Path.Segments) != 0 {
			return factflow.CallResultTarget{}, false
		}
		targetPath := target.Path
		if !target.HasPath {
			targetPath = path.NewPath(target.Symbol, "")
		}
		return factflow.NewCallResultTarget(factflow.CallResultTargetOrdinaryAssignment, target.Index, target.Symbol, targetPath), true
	case semantics.CallResultTargetReturn:
		return factflow.NewCallResultTarget(factflow.CallResultTargetReturn, target.Index, 0, path.Path{}), true
	default:
		return factflow.CallResultTarget{}, false
	}
}

func (l *lowerer) callSiteResultTargets(targets []semantics.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, len(targets))
	for i := range targets {
		out[i] = callSiteResultTarget(targets[i])
	}
	return out
}

func callSiteResultTarget(target semantics.CallResultTarget) factflow.CallResultTarget {
	targetKind := factflow.CallResultTargetUnknown
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		targetKind = factflow.CallResultTargetLocalAssignment
	case semantics.CallResultTargetOrdinaryAssignment:
		targetKind = factflow.CallResultTargetOrdinaryAssignment
	case semantics.CallResultTargetReturn:
		targetKind = factflow.CallResultTargetReturn
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
	return factflow.NewCallResultTarget(targetKind, target.Index, targetSymbol, targetPath)
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

func callProducerContext(kind semantics.CallContextKind) (factflow.CallProducerContext, bool) {
	switch kind {
	case semantics.CallContextAssignmentSource:
		return factflow.CallProducerContextAssignment, true
	case semantics.CallContextReturnSource:
		return factflow.CallProducerContextReturn, true
	default:
		return factflow.CallProducerContextUnknown, false
	}
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
	default:
		return factflow.CallSiteContextUnknown
	}
}

func valueSourceKind(kind valuesource.Kind) factflow.ValueSourceKind {
	switch kind {
	case valuesource.Expression:
		return factflow.ValueSourceExpression
	case valuesource.Call:
		return factflow.ValueSourceCall
	case valuesource.Vararg:
		return factflow.ValueSourceVararg
	case valuesource.Nil:
		return factflow.ValueSourceNil
	default:
		return factflow.ValueSourceUnknown
	}
}
