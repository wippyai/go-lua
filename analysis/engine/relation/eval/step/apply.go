package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	correlatedreplay "github.com/wippyai/go-lua/analysis/engine/relation/eval/replay"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// executeApply redeems the ordered child batch sets authored by the sealed
// Apply node. It does not flatten, zip, or otherwise assemble children: the
// Apply kernel owns the typed scalar/span alternatives, their cartesian
// frame product, scope conjunction, lineage, worker admission, and terminal
// outcomes.
func (session Session) executeApply(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Apply()
	if !ok || !binding.Available() {
		return nodeValue{}, false
	}
	if binding.Correlation().Specified() {
		values := make([]apply.Results, 0)
		completed, replayOK := correlatedreplay.Full(binding, session.mounted, session.root, session.geometry, session.scratch, func(_ correlatedreplay.CoordinateEvidence, result apply.Results) bool {
			values = append(values, result)
			return true
		})
		if !replayOK || !completed {
			return nodeValue{}, false
		}
		return applyNode(node.Digest(), values)
	}
	children, ok := session.children(node)
	if !ok || len(children) != binding.ChildCount() {
		return nodeValue{}, false
	}
	inputs := make([][]tuple.Batch, len(children))
	for index, child := range children {
		if !relationKind(child.kind) || child.batches == nil {
			return nodeValue{}, false
		}
		// Child position is the declaration position. Preserve its exact
		// non-nil extent: a zero-length batch vector is the closed no-selection
		// result of a valid child, whereas nil is an absent/malformed child
		// result. Only apply.Execute may compose vectors into frames.
		inputs[index] = make([]tuple.Batch, len(child.batches))
		copy(inputs[index], child.batches)
	}
	deliveries := binding.Deliveries()
	witnesses := make([]bindingpkg.DenominatorWitness, len(deliveries))
	for index, delivery := range deliveries {
		input := delivery.Requirement().Input()
		witnessValue, witnessOK := session.mounted.Denominator(input.Denominator)
		if !witnessOK || !witnessValue.ValidFor(session.mounted.RuntimeFence()) || !witnessValue.Matches(input.Denominator) {
			return nodeValue{}, false
		}
		witnesses[index] = witnessValue
	}
	values, executeOK := apply.Execute(binding, session.mounted, inputs, session.geometry, witness.Scope{}, witnesses)
	if !executeOK || !values.Available() {
		return nodeValue{}, false
	}
	return applyNode(node.Digest(), []apply.Results{values})
}
