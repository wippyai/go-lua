package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/operator/group"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeGroup partitions each sealed input range using the mounted Group
// binding. Group owns the key comparison and cardinality proof; evaluator
// code never scans or rebuilds a key directory.
func (session Session) executeGroup(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Group()
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
		values, executeOK := group.Execute(binding, session.mounted, source)
		if !executeOK || values == nil {
			return nodeValue{}, false
		}
		outputs = append(outputs, values...)
	}
	return relationNode(node.Digest(), algebra.KindGroup, outputs)
}
