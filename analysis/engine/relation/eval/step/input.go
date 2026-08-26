package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/operator/input"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeInput redeems one sealed Input plan. The Reader is bound to the
// plan's complete Values vector, while the Input kernel redeems its separate
// Scan layout only as range/cofiber authority. This is a physical redemption,
// not an evaluator-side schema scan or layout choice.
func (session Session) executeInput(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Input()
	if !ok || !binding.Available() {
		return nodeValue{}, false
	}
	reader, ok := read.Bind(session.root, binding.Values(), session.geometry, session.scratch)
	if !ok || !reader.Available() {
		return nodeValue{}, false
	}
	batches, ok := input.Execute(binding, session.mounted, reader)
	if !ok {
		return nodeValue{}, false
	}
	return relationNode(node.Digest(), algebra.KindInput, batches)
}
