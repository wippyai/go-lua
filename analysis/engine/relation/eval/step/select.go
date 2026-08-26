package step

import (
	selectop "github.com/wippyai/go-lua/analysis/engine/relation/operator/select"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeSelect interprets one sealed Select child over its ordered source
// ranges. Select has exactly one child; every source partition is retained as
// the same owner-issued range, including an authenticated empty partition.
func (session Session) executeSelect(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Select()
	if !ok || !binding.Available() {
		return nodeValue{}, false
	}
	children := node.Children()
	if len(children) != 1 {
		return nodeValue{}, false
	}
	child, ok := session.executeNode(children[0])
	if !ok || !child.available() || !relationKind(child.kind) {
		return nodeValue{}, false
	}
	outputs := make([]tuple.Batch, 0)
	for _, source := range child.batches {
		values, executeOK := selectop.Execute(binding, session.mounted, session.geometry, source)
		if !executeOK || values == nil {
			return nodeValue{}, false
		}
		outputs = append(outputs, values...)
	}
	return relationNode(node.Digest(), algebra.KindSelect, outputs)
}
