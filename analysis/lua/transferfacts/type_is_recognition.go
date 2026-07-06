package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) typeIsCall(fact semantics.CallFact) (typ.Type, path.Path, bool) {
	return l.typeIsCallExpr(fact.Call)
}

func typeRefPartsFromWIRPath(p path.Path) ([]string, bool) {
	if p.Root == "" {
		return nil, false
	}
	parts := make([]string, 0, 1+len(p.Segments))
	parts = append(parts, p.Root)
	for _, seg := range p.Segments {
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			if seg.Name == "" {
				return nil, false
			}
			parts = append(parts, seg.Name)
		default:
			return nil, false
		}
	}
	return parts, true
}

func (l *lowerer) typeIsCallExpr(call *ast.FuncCallExpr) (typ.Type, path.Path, bool) {
	call, receiver, ok := branchcond.TypeIsCallReceiver(call)
	if !ok {
		return nil, path.Path{}, false
	}
	t, ok := l.typeValueExpr(receiver)
	if !ok {
		return nil, path.Path{}, false
	}
	argPath, ok := pathexpr.Resolve(call.Args[0], l.bindings)
	if !ok || argPath.IsEmpty() {
		return nil, path.Path{}, false
	}
	return t, argPath, true
}

func (l *lowerer) addTypeIsBranchRefinements(input *factflow.FactsInput, graph cfg.Graph, result *semantics.Result) {
	if input == nil || graph == nil {
		return
	}
	if l != nil && l.wir != nil {
		l.addTypeIsBranchRefinementsFromWIR(input, graph)
		return
	}
	if result == nil {
		return
	}
	for _, callPoint := range graph.RPO() {
		view, ok := result.CallView(callPoint)
		if !ok {
			continue
		}
		fact, _ := view.Borrowed()
		t, argPath, ok := l.typeIsCall(fact)
		if !ok {
			continue
		}
		argValue := l.untrustedTypeWitnessValue(t)
		resultValue := l.typeIsProofValue(t)
		l.addTypeIsConditionBranchRefinements(input, graph, result, fact, argPath, argValue)
		errPath, ok := l.callResultTargetPath(callPoint, fact, 1)
		if !ok {
			continue
		}
		valuePath, hasValuePath := l.callResultTargetPath(callPoint, fact, 0)
		targets := typeIsTargets{argPath: argPath, errPath: errPath, valuePath: valuePath, hasValuePath: hasValuePath}
		establish, ok := typeIsEstablishPoint(input, graph, callPoint, targets)
		if !ok {
			continue
		}
		activeIn := typeIsActiveIn(input, graph, establish, targets)
		for _, branch := range graph.RPO() {
			if !activeIn[branch] || !graph.IsBranch(branch) {
				continue
			}
			edges := l.semanticTypeIsSuccessEdges(input, result, branch, errPath)
			if hasValuePath && len(edges) != 0 {
				appendBranchPresenceRelations(input.BranchPresenceRelations, branch,
					factflow.NewBranchPresenceRelation(errPath, presence.Present(), valuePath, presence.Absent()),
					factflow.NewBranchPresenceRelation(errPath, presence.Absent(), valuePath, presence.Present()),
				)
			}
			for _, cond := range edges {
				appendBranchRefinement(input.BranchRefinements, branch,
					branchRefinementOnEdge(argPath, factflow.NewValueConstraint(argValue), cond),
				)
				if hasValuePath {
					appendBranchRefinement(input.BranchRefinements, branch,
						branchRefinementOnEdge(valuePath, factflow.NewValueConstraint(resultValue), cond),
					)
				}
			}
		}
	}
}

func (l *lowerer) addTypeIsBranchRefinementsFromWIR(input *factflow.FactsInput, graph cfg.Graph) {
	if l == nil || l.wir == nil || input == nil || graph == nil {
		return
	}
	for _, callPoint := range graph.RPO() {
		site, ok := input.CallSites[callPoint]
		if !ok {
			continue
		}
		t, argPath, ok := l.typeIsCallSiteFromWIR(callPoint)
		if !ok {
			continue
		}
		argValue := l.untrustedTypeWitnessValue(t)
		resultValue := l.typeIsProofValue(t)
		if site.Context() == factflow.CallSiteContextCondition {
			l.addTypeIsConditionBranchRefinementsFromWIR(input, graph, callPoint, argPath, argValue)
		}
		errPath, ok := callSiteResultTargetPath(site, 1)
		if !ok {
			continue
		}
		valuePath, hasValuePath := callSiteResultTargetPath(site, 0)
		targets := typeIsTargets{argPath: argPath, errPath: errPath, valuePath: valuePath, hasValuePath: hasValuePath}
		establish, ok := typeIsEstablishPoint(input, graph, callPoint, targets)
		if !ok {
			continue
		}
		activeIn := typeIsActiveIn(input, graph, establish, targets)
		for _, branch := range graph.RPO() {
			if !activeIn[branch] || !graph.IsBranch(branch) {
				continue
			}
			edges := l.wirTypeIsSuccessEdges(input, branch, errPath)
			if hasValuePath && len(edges) != 0 {
				appendBranchPresenceRelations(input.BranchPresenceRelations, branch,
					factflow.NewBranchPresenceRelation(errPath, presence.Present(), valuePath, presence.Absent()),
					factflow.NewBranchPresenceRelation(errPath, presence.Absent(), valuePath, presence.Present()),
				)
			}
			for _, cond := range edges {
				appendBranchRefinement(input.BranchRefinements, branch,
					branchRefinementOnEdge(argPath, factflow.NewValueConstraint(argValue), cond),
				)
				if hasValuePath {
					appendBranchRefinement(input.BranchRefinements, branch,
						branchRefinementOnEdge(valuePath, factflow.NewValueConstraint(resultValue), cond),
					)
				}
			}
		}
	}
}

func (l *lowerer) typeIsCallSiteFromWIR(point cfg.Point) (typ.Type, path.Path, bool) {
	if l == nil || l.wir == nil || l.typeResolver == nil {
		return nil, path.Path{}, false
	}
	t, ok := l.typeIsReceiverTypeFromWIRCall(point)
	if !ok {
		return nil, path.Path{}, false
	}
	argPath, ok := l.callArgumentPathFromWIR(point, 0)
	if !ok {
		return nil, path.Path{}, false
	}
	return t, argPath, true
}

func (l *lowerer) typeIsReceiverTypeFromWIRCall(point cfg.Point) (typ.Type, bool) {
	if l == nil || l.wir == nil || l.typeResolver == nil {
		return nil, false
	}
	inst, ok := l.wirCallInstruction(point)
	if !ok {
		return nil, false
	}
	method := l.wir.Const(inst.Call.Method)
	if method.Kind != wir.ConstString || method.Str != "is" {
		return nil, false
	}
	if inst.Type != 0 {
		t := l.wir.Type(inst.Type)
		return t, t != nil
	}
	if inst.Call.Receiver.Kind == wir.OperandType {
		t := l.wir.Type(wir.TypeRef(inst.Call.Receiver.Ref))
		return t, t != nil
	}
	if inst.Call.Receiver.Kind != wir.OperandPath {
		return nil, false
	}
	receiverPath := l.wir.Path(wir.PathRef(inst.Call.Receiver.Ref))
	parts, ok := typeRefPartsFromWIRPath(receiverPath)
	if !ok {
		return nil, false
	}
	return l.typeResolver.ResolveTypeRef(parts)
}

func callSiteResultTargetPath(site factflow.CallSite, resultIndex int) (path.Path, bool) {
	for _, target := range site.ResultTargets() {
		if target.ResultIndex() != resultIndex {
			continue
		}
		targetPath := target.TargetPath()
		if targetPath.IsEmpty() {
			continue
		}
		return targetPath, true
	}
	return path.Path{}, false
}

func (l *lowerer) addTypeIsConditionBranchRefinements(
	input *factflow.FactsInput,
	graph cfg.Graph,
	result *semantics.Result,
	fact semantics.CallFact,
	argPath path.Path,
	value product.Value,
) {
	if fact.Context != semantics.CallContextCondition {
		return
	}
	for _, branch := range graph.RPO() {
		if l.wir != nil && !l.wir.HasInstruction(branch, wir.OpBranch) {
			continue
		}
		branchFact, ok := result.BranchCondition(branch)
		if !ok || branchFact.Stmt != fact.SourceStmt {
			continue
		}
		successCond, ok := typeIsConditionSuccessEdge(branchFact.Condition, fact.Call)
		if !ok {
			continue
		}
		appendBranchRefinement(input.BranchRefinements, branch,
			branchRefinementOnEdge(argPath, factflow.NewValueConstraint(value), successCond),
		)
	}
}

func (l *lowerer) addTypeIsConditionBranchRefinementsFromWIR(
	input *factflow.FactsInput,
	graph cfg.Graph,
	callPoint cfg.Point,
	argPath path.Path,
	value product.Value,
) {
	if l == nil || l.wir == nil {
		return
	}
	for _, branch := range graph.RPO() {
		if !l.wir.HasInstruction(branch, wir.OpBranch) {
			continue
		}
		successCond, ok := l.typeIsConditionSuccessEdgeFromWIR(branch, callPoint)
		if !ok {
			continue
		}
		appendBranchRefinement(input.BranchRefinements, branch,
			branchRefinementOnEdge(argPath, factflow.NewValueConstraint(value), successCond),
		)
	}
}

func (l *lowerer) typeIsConditionSuccessEdgeFromWIR(branch cfg.Point, callPoint cfg.Point) (bool, bool) {
	if l == nil || l.wir == nil {
		return false, false
	}
	for _, inst := range l.wir.PointInstructions(branch) {
		if inst.Op != wir.OpBranch {
			continue
		}
		if edge, ok := l.wirBranchOperandCallSuccessEdge(inst.A, callPoint); ok {
			return edge, true
		}
	}
	return false, false
}

func (l *lowerer) wirBranchOperandCallSuccessEdge(op wir.Operand, callPoint cfg.Point) (bool, bool) {
	if op.Kind != wir.OperandTemp {
		return false, false
	}
	def, ok := l.wirTempDefs()[op.Ref]
	if !ok {
		return false, false
	}
	if def.Op == wir.OpCall && def.Point == callPoint {
		return true, true
	}
	if def.Op == wir.OpUnOp && def.Operator == wir.UnNot {
		inner, ok := l.wirBranchOperandCallSuccessEdge(def.A, callPoint)
		return !inner, ok
	}
	return false, false
}

func branchRefinementOnEdge(target path.Path, value factflow.ValueRefinement, cond bool) factflow.BranchRefinement {
	if cond {
		return factflow.NewBranchRefinement(target, value, true, factflow.ValueRefinement{}, false)
	}
	return factflow.NewBranchRefinement(target, factflow.ValueRefinement{}, false, value, true)
}

func typeIsConditionSuccessEdge(condition ast.Expr, call *ast.FuncCallExpr) (bool, bool) {
	conditionCall, negated, ok := branchcond.PredicateCall(condition)
	if !ok || conditionCall != call {
		return false, false
	}
	if _, ok := branchcond.TypeIsCall(conditionCall); !ok {
		return false, false
	}
	return !negated, true
}

func (l *lowerer) typeIsExpressionConditionRefinement(expr ast.Expr) (factflow.PostconditionRefinement, bool, bool) {
	call, negated, ok := branchcond.PredicateCall(expr)
	if !ok {
		return factflow.PostconditionRefinement{}, false, false
	}
	t, argPath, ok := l.typeIsCallExpr(call)
	if !ok {
		return factflow.PostconditionRefinement{}, false, false
	}
	return factflow.NewPostconditionRefinement(
		argPath,
		factflow.NewValueConstraint(l.untrustedTypeWitnessValue(t)),
	), !negated, true
}

func (l *lowerer) typeIsCallResultValues(point cfg.Point, fact semantics.CallFact) []factflow.CallResultValue {
	t, _, ok := l.typeIsCall(fact)
	if !ok {
		return nil
	}
	return l.typeIsCallResultValuesForType(t)
}

func (l *lowerer) typeIsCallResultValuesFromWIR(point cfg.Point) []factflow.CallResultValue {
	t, _, ok := l.typeIsCallSiteFromWIR(point)
	if !ok {
		return nil
	}
	return l.typeIsCallResultValuesForType(t)
}

func (l *lowerer) typeIsCallResultValuesForType(t typ.Type) []factflow.CallResultValue {
	value := product.WithPresence(l.registry, l.typeIsProofValue(t), presence.Maybe())
	return []factflow.CallResultValue{
		factflow.NewCallResultValue(0, value),
		factflow.NewCallResultValue(1, product.Top()),
	}
}

func (l *lowerer) typeIsReturnPresenceRelationsFromSources(
	sources []factflow.ValueSource,
	result *semantics.Result,
	callSites map[cfg.Point]factflow.CallSite,
) []factflow.ReturnPresenceRelation {
	if len(sources) == 0 {
		return nil
	}
	isTypeIsCall := l.semanticSourceCallIsTypeIs(result)
	if l != nil && l.wir != nil {
		isTypeIsCall = l.wirSourceCallIsTypeIs(callSites)
	}
	var out []factflow.ReturnPresenceRelation
	type resultPair struct {
		valueTarget int
		hasValue    bool
		errorTarget int
		hasError    bool
	}
	pairs := make(map[cfg.Point]resultPair)
	for _, source := range sources {
		if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint {
			continue
		}
		if !isTypeIsCall(source) {
			continue
		}
		if source.OpenTail && source.Expanded {
			valueIndex := source.TargetIndex
			errorIndex := source.TargetIndex + 1
			out = append(out,
				factflow.NewReturnPresenceRelation(errorIndex, presence.Present(), valueIndex, presence.Absent()),
				factflow.NewReturnPresenceRelation(errorIndex, presence.Absent(), valueIndex, presence.Present()),
			)
			continue
		}
		pair := pairs[source.CallPoint]
		switch source.ResultIndex {
		case 0:
			pair.valueTarget = source.TargetIndex
			pair.hasValue = true
		case 1:
			pair.errorTarget = source.TargetIndex
			pair.hasError = true
		}
		pairs[source.CallPoint] = pair
	}
	for _, pair := range pairs {
		if !pair.hasValue || !pair.hasError {
			continue
		}
		out = append(out,
			factflow.NewReturnPresenceRelation(pair.errorTarget, presence.Present(), pair.valueTarget, presence.Absent()),
			factflow.NewReturnPresenceRelation(pair.errorTarget, presence.Absent(), pair.valueTarget, presence.Present()),
		)
	}
	return out
}

func (l *lowerer) wirSourceCallIsTypeIs(callSites map[cfg.Point]factflow.CallSite) func(factflow.ValueSource) bool {
	return func(source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint {
			return false
		}
		if _, ok := callSites[source.CallPoint]; !ok {
			return false
		}
		_, _, ok := l.typeIsCallSiteFromWIR(source.CallPoint)
		return ok
	}
}

func (l *lowerer) semanticSourceCallIsTypeIs(result *semantics.Result) func(factflow.ValueSource) bool {
	return func(source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || result == nil {
			return false
		}
		view, ok := result.CallView(source.CallPoint)
		if !ok {
			return false
		}
		fact, _ := view.Borrowed()
		_, _, ok = l.typeIsCall(fact)
		return ok
	}
}

func (l *lowerer) typeWitnessValue(t typ.Type) product.Value {
	return l.valueFromTypeWithWitness(t)
}

func (l *lowerer) typeIsProofValue(t typ.Type) product.Value {
	return l.runtimeAssertionValue(l.typeWitnessValue(t))
}

func (l *lowerer) runtimeAssertionValue(value product.Value) product.Value {
	return product.Set(l.registry, value, assertion.Key, assertion.Runtime())
}

func (l *lowerer) untrustedTypeWitnessValue(t typ.Type) product.Value {
	value := l.typeWitnessValue(t)
	return product.Set(l.registry, value, evidence.Key, evidence.ExplicitTop())
}

type typeIsTargets struct {
	argPath      path.Path
	errPath      path.Path
	valuePath    path.Path
	hasValuePath bool
}
