package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/operator/merge"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// executeMerge lowers all authored alternatives through the sealed Merge
// binding. The merge kernel owns key grouping and tuple reduction; this arm
// only preserves child order while collecting its immutable input ranges.
func (session Session) executeMerge(node arrangement.Node) (nodeValue, bool) {
	binding, ok := node.Merge()
	if !ok || !binding.Available() {
		return nodeValue{}, false
	}
	children, ok := session.children(node)
	if !ok || len(children) == 0 {
		return nodeValue{}, false
	}
	inputs := make([]tuple.Batch, 0)
	applications := make([]apply.Results, 0)
	proposalMerge := len(binding.ProposalOperations()) != 0
	for _, child := range children {
		if !relationKind(child.kind) && child.kind != algebra.KindApply {
			return nodeValue{}, false
		}
		if len(child.settlements) != 0 {
			return nodeValue{}, false
		}
		for _, results := range child.applications {
			if !proposalMerge || !results.Available() || !binding.AcceptsProposal(child.node, results.Operation()) {
				return nodeValue{}, false
			}
			applications = append(applications, results)
		}
		inputs = append(inputs, child.batches...)
	}
	if proposalMerge {
		// An Apply already holds the only proposal lease for its semantic
		// output. The carried relation is the prior destination fact and has
		// no new write of its own, so physical tuple reduction must not invent
		// a second proposal from it. Publish redeems the preserved applications
		// under its own sealed writable layout below.
		return composedRelationNode(node.Digest(), algebra.KindMerge, []tuple.Batch{}, applications)
	}
	if len(applications) != 0 {
		return nodeValue{}, false
	}
	outputs, executeOK := merge.Execute(binding, session.mounted, inputs)
	if !executeOK || outputs == nil {
		return nodeValue{}, false
	}
	return relationNode(node.Digest(), algebra.KindMerge, outputs)
}
