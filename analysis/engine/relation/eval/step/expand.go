package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/operator/expand"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeExpand redeems the mount-frozen C→P vector and applies its ordered
// R keys to the complete reader layout. The evaluator supplies only the
// committed state and sealed binding; it does not ask an owner for rows,
// derive a coordinate, or rebuild a vector.
func (session Session) executeExpand(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Expand()
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
	reader, ok := read.Bind(session.root, binding.Reader(), session.geometry, session.scratch)
	if !ok || !reader.Available() {
		return nodeValue{}, false
	}
	outputs := make([]tuple.Batch, 0, len(child.batches))
	for _, source := range child.batches {
		values, executeOK := expand.Execute(binding.Evidence(), session.mounted, session.geometry, source, reader)
		if !executeOK || values == nil {
			return nodeValue{}, false
		}
		outputs = append(outputs, values...)
	}
	return relationNode(node.Digest(), algebra.KindExpand, outputs)
}
