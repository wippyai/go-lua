package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) typeIsCall(fact semantics.CallFact) (typ.Type, path.Path, bool) {
	return l.typeIsCallExpr(fact.Call)
}

func (l *lowerer) typeIsCallExpr(call *ast.FuncCallExpr) (typ.Type, path.Path, bool) {
	call, ok := branchcond.TypeIsCall(call)
	if !ok {
		return nil, path.Path{}, false
	}
	t, ok := l.typeValueExpr(call.Receiver)
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
	if input == nil || graph == nil || result == nil {
		return
	}
	for _, callPoint := range graph.RPO() {
		fact, ok := result.Call(callPoint)
		if !ok {
			continue
		}
		t, argPath, ok := l.typeIsCall(fact)
		if !ok {
			continue
		}
		value := l.typeWitnessValue(t)
		l.addTypeIsConditionBranchRefinements(input, graph, result, fact, argPath, value)
		errPath, ok := callResultTargetPath(fact, 1)
		if !ok {
			continue
		}
		valuePath, hasValuePath := callResultTargetPath(fact, 0)
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
			edges := typeIsSuccessEdges(input, result, branch, errPath)
			if hasValuePath && len(edges) != 0 {
				appendBranchPresenceRelations(input.BranchPresenceRelations, branch,
					factflow.NewBranchPresenceRelation(errPath, presence.Present(), valuePath, presence.Absent()),
					factflow.NewBranchPresenceRelation(errPath, presence.Absent(), valuePath, presence.Present()),
				)
			}
			for _, cond := range edges {
				appendBranchRefinement(input.BranchRefinements, branch,
					branchRefinementOnEdge(argPath, factflow.NewValueConstraint(value), cond),
				)
				if hasValuePath {
					appendBranchRefinement(input.BranchRefinements, branch,
						branchRefinementOnEdge(valuePath, factflow.NewValueConstraint(value), cond),
					)
				}
			}
		}
	}
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
		factflow.NewValueConstraint(l.typeWitnessValue(t)),
	), !negated, true
}

func (l *lowerer) typeIsCallResultValues(fact semantics.CallFact) []factflow.CallResultValue {
	t, _, ok := l.typeIsCall(fact)
	if !ok {
		return nil
	}
	value := product.WithPresence(l.registry, l.typeWitnessValue(t), presence.Maybe())
	return []factflow.CallResultValue{
		factflow.NewCallResultValue(0, value),
		factflow.NewCallResultValue(1, product.Top()),
	}
}

func (l *lowerer) typeIsReturnPresenceRelations(sources []sourceprovenance.ASTSource, result *semantics.Result) []factflow.ReturnPresenceRelation {
	if len(sources) == 0 || result == nil {
		return nil
	}
	var out []factflow.ReturnPresenceRelation
	for _, source := range sources {
		if source.Kind != sourceprovenance.SourceCall || !source.OpenTail || !source.Expanded || !source.HasCallPoint {
			continue
		}
		fact, ok := result.Call(source.CallPoint)
		if !ok {
			continue
		}
		if _, _, ok := l.typeIsCall(fact); !ok {
			continue
		}
		valueIndex := source.TargetIndex
		errorIndex := source.TargetIndex + 1
		out = append(out,
			factflow.NewReturnPresenceRelation(errorIndex, presence.Present(), valueIndex, presence.Absent()),
			factflow.NewReturnPresenceRelation(errorIndex, presence.Absent(), valueIndex, presence.Present()),
		)
	}
	return out
}

func (l *lowerer) typeWitnessValue(t typ.Type) product.Value {
	return typevalue.WithWitness(l.registry, typevalue.FromType(l.registry, t), t)
}

type typeIsTargets struct {
	argPath      path.Path
	errPath      path.Path
	valuePath    path.Path
	hasValuePath bool
}

func callResultTargetPath(fact semantics.CallFact, resultIndex int) (path.Path, bool) {
	for _, target := range fact.ResultTargets {
		if target.ResultIndex == resultIndex && target.HasPath && !target.Path.IsEmpty() {
			return target.Path, true
		}
	}
	return path.Path{}, false
}
