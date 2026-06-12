package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) typeCastPostconditionRefinement(fact semantics.CallFact) (factflow.PostconditionRefinement, bool) {
	t, argPath, ok := l.directTypeCastCall(fact)
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(argPath, factflow.NewValueConstraint(l.typeWitnessValue(t))), true
}

func (l *lowerer) typeCastCallResultValue(fact semantics.CallFact) (factflow.CallResultValue, bool) {
	t, _, ok := l.directTypeCastCall(fact)
	if !ok {
		return factflow.CallResultValue{}, false
	}
	return factflow.NewCallResultValue(0, l.typeWitnessValue(t)), true
}

func (l *lowerer) directTypeCastCall(fact semantics.CallFact) (typ.Type, path.Path, bool) {
	if fact.Call == nil || fact.Receiver != nil || fact.Method != "" || len(fact.Args) != 1 || len(fact.TypeArgs) != 0 {
		return nil, path.Path{}, false
	}
	t, ok := l.typeValueExpr(fact.Func)
	if !ok {
		return nil, path.Path{}, false
	}
	argPath, ok := pathexpr.Resolve(fact.Args[0], l.bindings)
	if !ok || argPath.IsEmpty() {
		return nil, path.Path{}, false
	}
	return t, argPath, true
}

func (l *lowerer) typeIsCall(fact semantics.CallFact) (typ.Type, path.Path, bool) {
	if fact.Call == nil || fact.Receiver == nil || fact.Method != "is" || len(fact.Args) != 1 || len(fact.TypeArgs) != 0 {
		return nil, path.Path{}, false
	}
	t, ok := l.typeValueExpr(fact.Receiver)
	if !ok {
		return nil, path.Path{}, false
	}
	argPath, ok := pathexpr.Resolve(fact.Args[0], l.bindings)
	if !ok || argPath.IsEmpty() {
		return nil, path.Path{}, false
	}
	return t, argPath, true
}

func (l *lowerer) typeValueExpr(expr ast.Expr) (typ.Type, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || l.bindings == nil {
		return nil, false
	}
	decl, ok := l.bindings.TypeValueRef(ident)
	if !ok {
		return nil, false
	}
	return newTypeResolver(l.bindings).Decl(decl)
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
			for _, cond := range absentPresenceEdges(input, branch, errPath) {
				appendBranchRefinement(input.BranchRefinementSets, branch,
					factflow.NewBranchRefinement(argPath, factflow.NewValueConstraint(value), cond, factflow.ValueRefinement{}, false),
				)
				if hasValuePath {
					appendBranchRefinement(input.BranchRefinementSets, branch,
						factflow.NewBranchRefinement(valuePath, factflow.NewValueConstraint(value), cond, factflow.ValueRefinement{}, false),
					)
				}
			}
		}
	}
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

func typeIsEstablishPoint(input *factflow.FactsInput, graph cfg.Graph, callPoint cfg.Point, targets typeIsTargets) (cfg.Point, bool) {
	errAssign, ok := callResultAssignmentPoint(input, graph, callPoint, targets.errPath, 1)
	if !ok {
		return 0, false
	}
	if !targets.hasValuePath {
		return errAssign, true
	}
	valueAssign, ok := callResultAssignmentPoint(input, graph, callPoint, targets.valuePath, 0)
	if !ok {
		return errAssign, true
	}
	return laterPoint(graph, valueAssign, errAssign), true
}

func callResultAssignmentPoint(input *factflow.FactsInput, graph cfg.Graph, callPoint cfg.Point, target path.Path, resultIndex int) (cfg.Point, bool) {
	for _, point := range graph.RPO() {
		if local, ok := input.LocalAssignments[point]; ok &&
			local.TargetPath().Equal(target) &&
			valueSourceConsumesCallResult(local.Source(), callPoint, resultIndex) {
			return point, true
		}
		if ordinary, ok := input.OrdinaryAssignments[point]; ok &&
			ordinary.TargetPath().Equal(target) &&
			valueSourceConsumesCallResult(ordinary.Source(), callPoint, resultIndex) {
			return point, true
		}
	}
	return 0, false
}

func valueSourceConsumesCallResult(source factflow.ValueSource, callPoint cfg.Point, resultIndex int) bool {
	return source.Kind == factflow.ValueSourceCall &&
		source.HasCallPoint &&
		source.CallPoint == callPoint &&
		source.ResultIndex == resultIndex
}

func typeIsActiveIn(input *factflow.FactsInput, graph cfg.Graph, establish cfg.Point, targets typeIsTargets) map[cfg.Point]bool {
	rpo := graph.RPO()
	activeIn := make(map[cfg.Point]bool, len(rpo))
	activeOut := make(map[cfg.Point]bool, len(rpo))
	for changed := true; changed; {
		changed = false
		for _, point := range rpo {
			in := allPredecessorsActive(graph, point, activeOut)
			out := in
			switch {
			case point == establish:
				out = true
			case in && typeIsRelationKilledAt(input, point, targets):
				out = false
			}
			if activeIn[point] != in {
				activeIn[point] = in
				changed = true
			}
			if activeOut[point] != out {
				activeOut[point] = out
				changed = true
			}
		}
	}
	return activeIn
}

func typeIsRelationKilledAt(input *factflow.FactsInput, point cfg.Point, targets typeIsTargets) bool {
	if local, ok := input.LocalAssignments[point]; ok && typeIsKillsPath(local.TargetPath(), targets) {
		return true
	}
	if ordinary, ok := input.OrdinaryAssignments[point]; ok && typeIsKillsPath(ordinary.TargetPath(), targets) {
		return true
	}
	if pathAssign, ok := input.PathAssignments[point]; ok && typeIsKillsPath(pathAssign.TargetPath(), targets) {
		return true
	}
	return false
}

func typeIsKillsPath(candidate path.Path, targets typeIsTargets) bool {
	if candidate.Equal(targets.argPath) || candidate.Equal(targets.errPath) {
		return true
	}
	return targets.hasValuePath && candidate.Equal(targets.valuePath)
}

func absentPresenceEdges(input *factflow.FactsInput, branch cfg.Point, target path.Path) []bool {
	var out []bool
	if fact, ok := input.BranchRefinements[branch]; ok && fact.TargetPath().Equal(target) {
		out = appendAbsentPresenceEdges(out, fact)
	}
	if set, ok := input.BranchRefinementSets[branch]; ok {
		for _, fact := range set.Refinements() {
			if fact.TargetPath().Equal(target) {
				out = appendAbsentPresenceEdges(out, fact)
			}
		}
	}
	return out
}

func appendAbsentPresenceEdges(out []bool, fact factflow.BranchRefinement) []bool {
	if isAbsentRefinement(fact, true) {
		out = append(out, true)
	}
	if isAbsentRefinement(fact, false) {
		out = append(out, false)
	}
	return out
}

func isAbsentRefinement(fact factflow.BranchRefinement, cond bool) bool {
	refinement, ok := fact.ValueForEdge(cond)
	if !ok {
		return false
	}
	value, ok := refinement.Constraint()
	if !ok {
		return false
	}
	return presence.Equal(product.PresenceOf(value), presence.Absent())
}

func allPredecessorsActive(graph cfg.Graph, point cfg.Point, activeOut map[cfg.Point]bool) bool {
	preds := graph.Predecessors(point)
	if len(preds) == 0 {
		return false
	}
	for _, pred := range preds {
		if !activeOut[pred] {
			return false
		}
	}
	return true
}

func laterPoint(graph cfg.Graph, first, second cfg.Point) cfg.Point {
	order := make(map[cfg.Point]int, len(graph.RPO()))
	for i, point := range graph.RPO() {
		order[point] = i
	}
	if order[second] > order[first] {
		return second
	}
	return first
}

func appendPostconditionRefinements(out map[cfg.Point]factflow.PostconditionRefinementSet, point cfg.Point, refinements ...factflow.PostconditionRefinement) {
	if len(refinements) == 0 {
		return
	}
	existing := out[point].Refinements()
	existing = append(existing, refinements...)
	out[point] = factflow.NewPostconditionRefinementSet(existing...)
}

func appendBranchRefinement(out map[cfg.Point]factflow.BranchRefinementSet, point cfg.Point, refinements ...factflow.BranchRefinement) {
	if len(refinements) == 0 {
		return
	}
	existing := out[point].Refinements()
	existing = append(existing, refinements...)
	out[point] = factflow.NewBranchRefinementSet(existing...)
}
