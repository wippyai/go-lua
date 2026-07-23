package transformer

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// indexRelationCallReceivers derives the ordered call-result assignment lens
// directly from canonical relationCode. No materialization schedule owns this
// metadata: the root-assignment transaction is already the semantic source.
func indexRelationCallReceivers(code *relationCode) map[cfg.Point][]rootAssignmentTerm {
	if code == nil {
		return nil
	}
	type receiver struct {
		target int
		term   rootAssignmentTerm
	}
	grouped := make(map[cfg.Point][]receiver)
	seen := make(map[cfg.Point]struct{})
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		for _, step := range code.nodes[ref].steps {
			if step.kind != boundaryStepRootAssignment || !step.rootAssignment.structurallyValid() {
				continue
			}
			point := step.rootAssignment.transaction.Point()
			if _, duplicate := seen[point]; duplicate {
				continue
			}
			source, ok := step.rootAssignment.transaction.Source(0)
			if !ok || source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.TargetIndex < 0 {
				continue
			}
			seen[point] = struct{}{}
			grouped[source.CallPoint] = append(grouped[source.CallPoint], receiver{target: source.TargetIndex, term: step.rootAssignment})
		}
	}
	out := make(map[cfg.Point][]rootAssignmentTerm, len(grouped))
	for point, receivers := range grouped {
		sort.Slice(receivers, func(i, j int) bool {
			if receivers[i].target != receivers[j].target {
				return receivers[i].target < receivers[j].target
			}
			return receivers[i].term.transaction.Point() < receivers[j].term.transaction.Point()
		})
		terms := make([]rootAssignmentTerm, len(receivers))
		for index := range receivers {
			terms[index] = receivers[index].term
		}
		out[point] = terms
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func relationCallReceiverPoint(code *relationCode, callPoint cfg.Point, targetIndex int) (cfg.Point, bool) {
	if code == nil || callPoint == 0 || targetIndex < 0 {
		return 0, false
	}
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		for _, step := range code.nodes[ref].steps {
			if step.kind != boundaryStepRootAssignment || !step.rootAssignment.structurallyValid() {
				continue
			}
			source, ok := step.rootAssignment.transaction.Source(0)
			if ok && source.Kind == factflow.ValueSourceCall && source.HasCallPoint && source.CallPoint == callPoint && source.TargetIndex == targetIndex {
				return step.rootAssignment.transaction.Point(), true
			}
		}
	}
	return 0, false
}

// relationCallResultCarrierPaths returns every structural carrier belonging to
// one point-owned call-result scalar. A finite multi-target call has one path
// partition per linked frame occurrence but one scalar producer identity.
// Deriving the paths from the sealed call-frame inventory keeps this usable
// while frames themselves are being linked; no link-order dependency exists.
func relationCallResultCarrierPaths(body *relationProgramBody, slot key.Value) ([]keyspace.Key, error) {
	point, result, exact := key.ParseCallResult(slot)
	if !exact || body == nil || body.relation.arena == nil || body.keys == nil || !body.keys.Valid() {
		return nil, fmt.Errorf("transformer: call-result structural source has no body authority")
	}
	paths := make([]keyspace.Key, 0, 1)
	for term := callFrameTerm(1); int(term) < len(body.relation.arena.callFrames); term++ {
		frame := body.relation.arena.callFrames[term]
		if uint32(frame.point) != point || result >= frame.resultCount {
			continue
		}
		_, path, err := frameCallResultCarrier(body.keys, body.body, frame.point, result)
		if err != nil {
			return nil, err
		}
		duplicate := false
		for _, prior := range paths {
			duplicate = duplicate || prior == path
		}
		if !duplicate {
			paths = append(paths, path)
		}
	}
	// A point-owned scalar may be produced by a non-lexical signature/module
	// call. Such producers have no lexical frame and therefore no frame-owned
	// descendant path to transport here; their explicit producer transaction
	// owns any structural publication. The empty path set is exact.
	if len(paths) == 0 {
		return nil, nil
	}
	sort.Slice(paths, func(i, j int) bool { return body.keys.Less(paths[i], paths[j]) })
	return paths, nil
}

// linkRelationFrameResultSources derives addressable return-source closure
// roots from the immutable outcome tuples. Scalar result identity remains the
// canonical frame result selector and is never duplicated here.
func linkRelationFrameResultSources(target *relationProgramBody, code *relationCode, results []linkedFrameResult) ([]linkedFrameResultSource, error) {
	if target == nil || target.relation.arena == nil || target.keys == nil || code == nil {
		return nil, fmt.Errorf("transformer: result-source lens has no target relation authority")
	}
	seen := make(map[linkedFrameResultSource]struct{})
	out := make([]linkedFrameResultSource, 0)
	for outcomeRef := boundaryOutcomeRef(1); int(outcomeRef) < len(code.outcomes); outcomeRef++ {
		transaction := code.outcomes[outcomeRef].returnTransaction
		point := transaction.transaction.Point()
		for binding := 0; binding < transaction.transaction.ResultBindingCount(); binding++ {
			sourceIndex, resultIndex, ok := transaction.transaction.ResultBinding(binding)
			if !ok || resultIndex < 0 || resultIndex >= len(results) || !linkedResultHasAddressTarget(results[resultIndex]) || sourceIndex < 0 || sourceIndex >= len(transaction.sources) {
				continue
			}
			term := transaction.sources[sourceIndex]
			if term == 0 || int(term) >= len(target.relation.arena.values) {
				continue
			}
			node := target.relation.arena.values[term]
			source := linkedFrameResultSource{result: uint32(resultIndex), value: term}
			var sources []linkedFrameResultSource
			switch node.op {
			case valueEnvironment:
				if _, scratch := key.ParseExpressionValue(node.slot); scratch {
					continue
				}
				source.slot = node.slot
				if _, _, callResult := key.ParseCallResult(node.slot); callResult {
					paths, pathErr := relationCallResultCarrierPaths(target, node.slot)
					if pathErr != nil {
						return nil, pathErr
					}
					for _, path := range paths {
						copy := source
						copy.path = path
						sources = append(sources, copy)
					}
					break
				}
				if symbol, present := key.ParseSymbolValue(node.slot); present && target.pathSemantics != nil {
					source.path, _ = target.pathSemantics.VisibleLocalPathKey(point, pathdom.NewPath(symbol, ""))
				}
			case valueRoot:
				for _, carrier := range target.roots.roots {
					if carrier.root != node.root {
						continue
					}
					source.slot = carrier.slot
					if symbol := rootSymbol(carrier.slot); symbol != 0 && target.pathSemantics != nil {
						source.path, _ = target.pathSemantics.VisibleLocalPathKey(point, pathdom.NewPath(symbol, ""))
					}
					break
				}
			}
			if len(sources) == 0 {
				sources = append(sources, source)
			}
			for _, candidate := range sources {
				if candidate.slot == 0 && candidate.path.Kind == keyspace.KindInvalid || candidate.path.Kind != keyspace.KindInvalid && target.keys.FormatReadOnly(candidate.path) == "" {
					continue
				}
				if _, duplicate := seen[candidate]; duplicate {
					continue
				}
				seen[candidate] = struct{}{}
				out = append(out, candidate)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].result != out[j].result {
			return out[i].result < out[j].result
		}
		if out[i].path != out[j].path {
			return target.keys.Less(out[i].path, out[j].path)
		}
		return out[i].slot < out[j].slot
	})
	return out, nil
}
