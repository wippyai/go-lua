package pathevidence

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statepathkey "github.com/wippyai/go-lua/analysis/domain/state/pathkey"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// InvalidatePathKeySubtree removes finite path evidence at pathKey and any
// descendant key. It returns false when pathKey is not a recognized structural
// path-key spelling.
func (l Lane) InvalidatePathKeySubtree(pathKey pathdom.PathKey) (Lane, bool) {
	if l.refinementsTop {
		panic("state: cannot invalidate path subtree in top path lane")
	}
	refinements, changed, ok := statepathkey.DeleteSubtree(l.refinements, pathKey)
	if !ok {
		return l, false
	}
	staticMembers, staticChanged, _ := statepathkey.DeleteSubtree(l.staticMembers, pathKey)
	proofs, proofChanged := deleteBranchProofsMatching(l.proofs, func(candidate pathdom.PathKey) bool {
		return pathKeyInSubtree(candidate, pathKey)
	})
	if !changed && !staticChanged && !proofChanged {
		return l, true
	}
	out := l
	out.refinements = refinements
	out.staticMembers = staticMembers
	out.proofs = proofs
	return out, true
}

// InvalidatePathKeyDescendants removes finite path evidence below pathKey while
// preserving exact pathKey evidence. It returns false when pathKey is not a
// recognized structural path-key spelling.
func (l Lane) InvalidatePathKeyDescendants(pathKey pathdom.PathKey) (Lane, bool) {
	if l.refinementsTop {
		panic("state: cannot invalidate path descendants in top path lane")
	}
	refinements, changed, ok := statepathkey.DeleteDescendants(l.refinements, pathKey)
	if !ok {
		return l, false
	}
	staticMembers, staticChanged, _ := statepathkey.DeleteDescendants(l.staticMembers, pathKey)
	proofs, proofChanged := deleteBranchProofsMatching(l.proofs, func(candidate pathdom.PathKey) bool {
		return pathKeyInDescendants(candidate, pathKey)
	})
	if !changed && !staticChanged && !proofChanged {
		return l, true
	}
	out := l
	out.refinements = refinements
	out.staticMembers = staticMembers
	out.proofs = proofs
	return out, true
}

func pathKeyInSubtree(candidate pathdom.PathKey, prefix pathdom.PathKey) bool {
	if candidate == "" {
		return false
	}
	_, changed, ok := statepathkey.DeleteSubtree(map[pathdom.PathKey]product.Value{candidate: {}}, prefix)
	return ok && changed
}

func pathKeyInDescendants(candidate pathdom.PathKey, prefix pathdom.PathKey) bool {
	if candidate == "" {
		return false
	}
	_, changed, ok := statepathkey.DeleteDescendants(map[pathdom.PathKey]product.Value{candidate: {}}, prefix)
	return ok && changed
}
