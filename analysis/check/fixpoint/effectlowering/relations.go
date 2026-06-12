package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SignatureRelationConfig carries the signature lookup and generic fact model
// needed to lower declared signature relation effects.
type SignatureRelationConfig struct {
	Graph      cfg.Graph
	Signatures SignatureLookup
	NameFor    SignatureNameFunc
	Facts      factflow.Facts
}

// WithSignatureRelations returns Facts extended with generic relation facts
// lowered from declared signature effects.
func WithSignatureRelations(config SignatureRelationConfig) factflow.Facts {
	if config.Graph == nil || config.Signatures == nil || config.NameFor == nil {
		return config.Facts
	}
	relations := signatureRelationFacts(config)
	return config.Facts.WithBranchPresenceRelations(relations)
}

type errorReturnTargets struct {
	valueTarget factflow.CallResultTarget
	errorTarget factflow.CallResultTarget
	valueAssign cfg.Point
	errorAssign cfg.Point
	establish   cfg.Point
}

func signatureRelationFacts(config SignatureRelationConfig) map[cfg.Point]factflow.BranchPresenceRelationSet {
	out := make(map[cfg.Point]factflow.BranchPresenceRelationSet)
	for _, point := range config.Graph.RPO() {
		call, ok := config.Facts.Call(point)
		if !ok {
			continue
		}
		sig, ok := signatureForCall(config, point, call)
		if !ok {
			continue
		}
		for _, label := range activeErrorReturnLabels(sig) {
			targets, ok := errorReturnCallTargets(config, point, call, label)
			if !ok {
				continue
			}
			addErrorReturnRelations(config, out, targets)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func signatureForCall(config SignatureRelationConfig, point cfg.Point, call factflow.CallProducer) (signature.Function, bool) {
	name, ok := config.NameFor(transfer.NodeContext{
		Graph: config.Graph,
		Point: point,
		Node:  config.Graph.Node(point),
	}, call)
	if !ok {
		return signature.Function{}, false
	}
	return config.Signatures.Lookup(name)
}

func activeErrorReturnLabels(sig signature.Function) []returns.ErrorReturn {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]returns.ErrorReturn, 0, len(sig.Effect.Labels))
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case returns.ErrorReturn:
			out = append(out, normalized)
		case *returns.ErrorReturn:
			if normalized != nil {
				out = append(out, *normalized)
			}
		}
	}
	return out
}

func errorReturnCallTargets(
	config SignatureRelationConfig,
	callPoint cfg.Point,
	call factflow.CallProducer,
	label returns.ErrorReturn,
) (errorReturnTargets, bool) {
	valueTarget, ok := callTargetForResult(call, label.ValueIndex)
	if !ok || !relatableCallTarget(valueTarget) {
		return errorReturnTargets{}, false
	}
	errorTarget, ok := callTargetForResult(call, label.ErrorIndex)
	if !ok || !relatableCallTarget(errorTarget) {
		return errorReturnTargets{}, false
	}
	valueAssign, ok := callResultAssignmentPoint(config.Graph, config.Facts, callPoint, valueTarget, label.ValueIndex)
	if !ok {
		return errorReturnTargets{}, false
	}
	errorAssign, ok := callResultAssignmentPoint(config.Graph, config.Facts, callPoint, errorTarget, label.ErrorIndex)
	if !ok {
		return errorReturnTargets{}, false
	}
	return errorReturnTargets{
		valueTarget: valueTarget,
		errorTarget: errorTarget,
		valueAssign: valueAssign,
		errorAssign: errorAssign,
		establish:   laterPoint(config.Graph, valueAssign, errorAssign),
	}, true
}

func callTargetForResult(call factflow.CallProducer, resultIndex int) (factflow.CallResultTarget, bool) {
	if resultIndex < 0 {
		return factflow.CallResultTarget{}, false
	}
	for _, target := range call.ResultTargets() {
		if target.ResultIndex() == resultIndex {
			return target, true
		}
	}
	return factflow.CallResultTarget{}, false
}

func relatableCallTarget(target factflow.CallResultTarget) bool {
	switch target.Kind() {
	case factflow.CallResultTargetLocalAssignment, factflow.CallResultTargetOrdinaryAssignment:
		return target.TargetSymbol() != 0 && !target.TargetPath().IsEmpty()
	default:
		return false
	}
}

func callResultAssignmentPoint(
	graph cfg.Graph,
	facts factflow.Facts,
	callPoint cfg.Point,
	target factflow.CallResultTarget,
	resultIndex int,
) (cfg.Point, bool) {
	for _, point := range graph.RPO() {
		if assignment, ok := facts.RootAssignment(point); ok &&
			assignment.TargetPath().Equal(target.TargetPath()) &&
			valueSourceConsumesCallResult(assignment.Source(), callPoint, target, resultIndex) {
			return point, true
		}
	}
	return 0, false
}

func valueSourceConsumesCallResult(
	source factflow.ValueSource,
	callPoint cfg.Point,
	target factflow.CallResultTarget,
	resultIndex int,
) bool {
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != callPoint {
		return false
	}
	if source.ResultIndex != resultIndex {
		return false
	}
	return source.TargetIndex == target.Index()
}

func laterPoint(graph cfg.Graph, first, second cfg.Point) cfg.Point {
	order := pointOrder(graph)
	if order[second] > order[first] {
		return second
	}
	return first
}

func addErrorReturnRelations(
	config SignatureRelationConfig,
	out map[cfg.Point]factflow.BranchPresenceRelationSet,
	targets errorReturnTargets,
) {
	activeIn := relationActiveIn(config.Graph, config.Facts, targets)
	for _, point := range config.Graph.RPO() {
		if !activeIn[point] || !config.Graph.IsBranch(point) || !branchRefinesPath(config.Facts, point, targets.errorTarget) {
			continue
		}
		appendBranchPresenceRelations(out, point,
			factflow.NewBranchPresenceRelation(
				targets.errorTarget.TargetPath(),
				presence.Present(),
				targets.valueTarget.TargetPath(),
				presence.Absent(),
			),
			factflow.NewBranchPresenceRelation(
				targets.errorTarget.TargetPath(),
				presence.Absent(),
				targets.valueTarget.TargetPath(),
				presence.Present(),
			),
		)
	}
}

func relationActiveIn(graph cfg.Graph, facts factflow.Facts, targets errorReturnTargets) map[cfg.Point]bool {
	rpo := graph.RPO()
	activeIn := make(map[cfg.Point]bool, len(rpo))
	activeOut := make(map[cfg.Point]bool, len(rpo))
	for changed := true; changed; {
		changed = false
		for _, point := range rpo {
			in := allPredecessorsActive(graph, point, activeOut)
			out := in
			switch {
			case point == targets.establish:
				out = true
			case in && relationKilledAt(facts, point, targets):
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

func relationKilledAt(facts factflow.Facts, point cfg.Point, targets errorReturnTargets) bool {
	if point == targets.valueAssign || point == targets.errorAssign {
		return false
	}
	if assignment, ok := facts.RootAssignment(point); ok && relationTargetPath(assignment.TargetPath(), targets) {
		return true
	}
	if pathAssignment, ok := facts.PathAssignment(point); ok && relationTargetPath(pathAssignment.TargetPath(), targets) {
		return true
	}
	return false
}

func relationTargetPath(targetPath pathdom.Path, targets errorReturnTargets) bool {
	return targetPath.Equal(targets.valueTarget.TargetPath()) || targetPath.Equal(targets.errorTarget.TargetPath())
}

func branchRefinesPath(facts factflow.Facts, point cfg.Point, target factflow.CallResultTarget) bool {
	targetPath := target.TargetPath()
	for _, refinement := range facts.BranchRefinements(point) {
		if refinement.TargetPath().Equal(targetPath) {
			return true
		}
	}
	return false
}

func appendBranchPresenceRelations(
	out map[cfg.Point]factflow.BranchPresenceRelationSet,
	point cfg.Point,
	relations ...factflow.BranchPresenceRelation,
) {
	existing := out[point].Relations()
	existing = append(existing, relations...)
	out[point] = factflow.NewBranchPresenceRelationSet(existing...)
}

func pointOrder(graph cfg.Graph) map[cfg.Point]int {
	rpo := graph.RPO()
	out := make(map[cfg.Point]int, len(rpo))
	for i, point := range rpo {
		out[point] = i
	}
	return out
}
