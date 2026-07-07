package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

type resultCorrelationTargets struct {
	triggerPath        path.Path
	triggerResultIndex int
	targetPath         path.Path
	targetResultIndex  int
	hasTargetPath      bool
	killPaths          []path.Path
}

func typeIsEstablishPoint(input *factflow.FactsInput, graph cfg.Graph, callPoint cfg.Point, targets typeIsTargets) (cfg.Point, bool) {
	return resultCorrelationEstablishPoint(input, graph, callPoint, typeIsResultCorrelationTargets(targets))
}

func typeIsResultCorrelationTargets(targets typeIsTargets) resultCorrelationTargets {
	return resultCorrelationTargets{
		triggerPath:        targets.errPath,
		triggerResultIndex: 1,
		targetPath:         targets.valuePath,
		targetResultIndex:  0,
		hasTargetPath:      targets.hasValuePath,
		killPaths:          []path.Path{targets.argPath},
	}
}

func resultCorrelationEstablishPoint(input *factflow.FactsInput, graph cfg.Graph, callPoint cfg.Point, targets resultCorrelationTargets) (cfg.Point, bool) {
	triggerAssign, ok := callResultAssignmentPoint(input, graph, callPoint, targets.triggerPath, targets.triggerResultIndex)
	if !ok {
		return 0, false
	}
	if !targets.hasTargetPath {
		return triggerAssign, true
	}
	targetAssign, ok := callResultAssignmentPoint(input, graph, callPoint, targets.targetPath, targets.targetResultIndex)
	if !ok {
		return triggerAssign, true
	}
	return laterPoint(graph, targetAssign, triggerAssign), true
}

func callResultAssignmentPoint(input *factflow.FactsInput, graph cfg.Graph, callPoint cfg.Point, target path.Path, resultIndex int) (cfg.Point, bool) {
	for _, point := range graph.RPO() {
		if assignment, ok := input.RootAssignments[point]; ok &&
			assignment.TargetPath().Equal(target) &&
			valueSourceConsumesCallResult(assignment.Source(), callPoint, resultIndex) {
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
	return resultCorrelationActiveIn(input, graph, establish, typeIsResultCorrelationTargets(targets))
}

func resultCorrelationActiveIn(input *factflow.FactsInput, graph cfg.Graph, establish cfg.Point, targets resultCorrelationTargets) map[cfg.Point]bool {
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
			case in && resultCorrelationKilledAt(input, point, targets):
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

func resultCorrelationKilledAt(input *factflow.FactsInput, point cfg.Point, targets resultCorrelationTargets) bool {
	if assignment, ok := input.RootAssignments[point]; ok && resultCorrelationKillsPath(assignment.TargetPath(), targets) {
		return true
	}
	if pathAssign, ok := input.PathAssignments[point]; ok && resultCorrelationKillsPath(pathAssign.TargetPath(), targets) {
		return true
	}
	return false
}

func resultCorrelationKillsPath(candidate path.Path, targets resultCorrelationTargets) bool {
	if candidate.Equal(targets.triggerPath) {
		return true
	}
	if targets.hasTargetPath && candidate.Equal(targets.targetPath) {
		return true
	}
	for _, kill := range targets.killPaths {
		if candidate.Equal(kill) {
			return true
		}
	}
	return false
}

func absentPresenceEdges(input *factflow.FactsInput, branch cfg.Point, target path.Path) []bool {
	var out []bool
	if set, ok := input.BranchRefinements[branch]; ok {
		for _, fact := range set.Refinements() {
			if fact.TargetPath().Equal(target) {
				out = appendAbsentPresenceEdges(out, fact)
			}
		}
	}
	return out
}

func (l *lowerer) wirTypeIsSuccessEdges(input *factflow.FactsInput, branch cfg.Point, target path.Path) []bool {
	out := absentPresenceEdges(input, branch, target)
	if check, ok := l.directBranchCheckFromWIR(branch); ok &&
		check.Kind == branchcond.CheckFalsy &&
		check.Path.Equal(target) {
		out = appendBoolIfMissing(out, true)
	}
	return out
}

func appendBoolIfMissing(out []bool, value bool) []bool {
	for _, existing := range out {
		if existing == value {
			return out
		}
	}
	return append(out, value)
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
	preds := cfg.PredecessorsReadOnly(graph, point)
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
