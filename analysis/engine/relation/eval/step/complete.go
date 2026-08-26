package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/operator/complete"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeComplete closes every source cofiber against the binding's sealed
// denominator. Missing members and their typed absence cells are issued by
// the complete owner; the evaluator only transports the resulting batches.
func (session Session) executeComplete(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Complete()
	if !ok || !binding.Available() {
		return nodeValue{}, false
	}
	children := node.Children()
	if len(children) != 1 {
		return nodeValue{}, false
	}
	// Resolve the ordinary global denominator witness once at the evaluator
	// boundary. Correlated replay does not use this path: it supplies its
	// q-specific posting directly to complete.Execute.
	denominatorWitness, witnessOK := session.mounted.Denominator(binding.Denominator())
	if !witnessOK || !denominatorWitness.ValidFor(session.mounted.RuntimeFence()) || !denominatorWitness.Matches(binding.Denominator()) {
		return nodeValue{}, false
	}
	child, ok := session.executeNode(children[0])
	if !ok || !child.available() || !relationKind(child.kind) {
		return nodeValue{}, false
	}
	outputs := make([]tuple.Batch, 0, len(child.batches))
	for _, source := range child.batches {
		value, executeOK := complete.Execute(binding, session.mounted, source, denominatorWitness)
		if !executeOK || !value.Available() {
			return nodeValue{}, false
		}
		outputs = append(outputs, value)
	}
	return relationNode(node.Digest(), algebra.KindComplete, outputs)
}
