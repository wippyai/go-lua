package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeColumnProject redeems the closed cell ordinals bound at mount. It
// preserves each input range proof, because choosing semantic output cells
// does not create a new row range. If the child carries Apply proposal
// sidecars, they remain sealed sidecars for the enclosing Publish; the
// physical tuple projection itself never searches a proposal by column name.
func (session Session) executeColumnProject(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.ColumnProject()
	if !ok || !binding.Available() {
		return nodeValue{}, false
	}
	children := node.Children()
	if len(children) != 1 {
		return nodeValue{}, false
	}
	child, ok := session.executeNode(children[0])
	if !ok || !child.available() || !relationKind(child.kind) && child.kind != algebra.KindApply {
		return nodeValue{}, false
	}
	projected := make([]tuple.Batch, len(child.batches))
	for batchIndex, batch := range child.batches {
		if !batch.ValidFor(session.mounted) {
			return nodeValue{}, false
		}
		values := make([]tuple.Tuple, batch.Len())
		for tupleIndex := 0; tupleIndex < batch.Len(); tupleIndex++ {
			value, valueOK := batch.At(tupleIndex)
			if !valueOK {
				return nodeValue{}, false
			}
			value, valueOK = tuple.ProjectColumns(session.mounted, value, binding)
			if !valueOK {
				return nodeValue{}, false
			}
			values[tupleIndex] = value
		}
		output, outputOK := tuple.PreserveRange(session.mounted, batch, batch.Scope(), values)
		if !outputOK {
			return nodeValue{}, false
		}
		projected[batchIndex] = output
	}
	if len(child.applications) != 0 {
		return composedRelationNode(node.Digest(), algebra.KindColumnProject, projected, child.applications)
	}
	return relationNode(node.Digest(), algebra.KindColumnProject, projected)
}
