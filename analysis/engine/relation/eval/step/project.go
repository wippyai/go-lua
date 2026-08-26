package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/operator/project"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeProject redeems one sealed source-to-target mapping. The target
// reader is bound from ProjectBinding.Target(), the complete target-row
// layout selected by mount; no runtime access, mapping, or destination index
// is reconstructed here. ProjectBinding.Key remains the independent
// equality/index access.
func (session Session) executeProject(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Project()
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
	// Project needs the complete destination row tuple. Binding.Key remains
	// sealed in ProjectBinding for key equality, but a key access carries no
	// delivered cells and cannot authenticate the target occurrence.
	reader, ok := read.Bind(session.root, binding.Target(), session.geometry, session.scratch)
	if !ok || !reader.Available() {
		return nodeValue{}, false
	}
	outputs := make([]tuple.Batch, 0)
	for _, source := range child.batches {
		values, executeOK := project.Execute(binding, session.mounted, source, reader)
		if !executeOK || values == nil {
			return nodeValue{}, false
		}
		outputs = append(outputs, values...)
	}
	return relationNode(node.Digest(), algebra.KindProject, outputs)
}
