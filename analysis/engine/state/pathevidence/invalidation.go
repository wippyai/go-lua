package pathevidence

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// InvalidatePathKeySubtree removes finite path evidence at pathKey and any
// descendant key. It returns false when pathKey is not a recognized structural
// path-key spelling.
func (l Lane) InvalidatePathKeySubtree(pathKey pathdom.PathKey) (Lane, bool) {
	if l.refinementsTop {
		panic("state: cannot invalidate path subtree in top path lane")
	}
	refinements, changed, ok := deletePathKeySubtree(l.refinements, pathKey)
	if !ok {
		return l, false
	}
	staticMembers, staticChanged, _ := deletePathKeySubtree(l.staticMembers, pathKey)
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
	refinements, changed, ok := deletePathKeyDescendants(l.refinements, pathKey)
	if !ok {
		return l, false
	}
	staticMembers, staticChanged, _ := deletePathKeyDescendants(l.staticMembers, pathKey)
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
	parsed, parsedPrefix, ok := parsePathKeyPair(candidate, prefix)
	return ok && parsed.HasPrefix(parsedPrefix)
}

func pathKeyInDescendants(candidate pathdom.PathKey, prefix pathdom.PathKey) bool {
	parsed, parsedPrefix, ok := parsePathKeyPair(candidate, prefix)
	return ok && parsed.HasStrictPrefix(parsedPrefix)
}

func deletePathKeySubtree(
	in map[pathdom.PathKey]product.Value,
	prefix pathdom.PathKey,
) (map[pathdom.PathKey]product.Value, bool, bool) {
	parsedPrefix, ok := pathaddr.StructuralKeyFromPathKey(prefix)
	if !ok {
		return in, false, false
	}
	out, changed := deleteMatchingPathKeys(in, func(candidate pathaddr.StructuralKey) bool {
		return candidate.HasPrefix(parsedPrefix)
	})
	return out, changed, true
}

func deletePathKeyDescendants(
	in map[pathdom.PathKey]product.Value,
	prefix pathdom.PathKey,
) (map[pathdom.PathKey]product.Value, bool, bool) {
	parsedPrefix, ok := pathaddr.StructuralKeyFromPathKey(prefix)
	if !ok {
		return in, false, false
	}
	out, changed := deleteMatchingPathKeys(in, func(candidate pathaddr.StructuralKey) bool {
		return candidate.HasStrictPrefix(parsedPrefix)
	})
	return out, changed, true
}

func deleteMatchingPathKeys(
	in map[pathdom.PathKey]product.Value,
	match func(pathaddr.StructuralKey) bool,
) (map[pathdom.PathKey]product.Value, bool) {
	if len(in) == 0 {
		return in, false
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
	changed := false
	for pathKey, value := range in {
		parsed, ok := pathaddr.StructuralKeyFromPathKey(pathKey)
		if ok && match(parsed) {
			changed = true
			continue
		}
		out[pathKey] = value
	}
	if !changed {
		return in, false
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

func parsePathKeyPair(candidate pathdom.PathKey, prefix pathdom.PathKey) (pathaddr.StructuralKey, pathaddr.StructuralKey, bool) {
	if candidate == "" {
		return pathaddr.StructuralKey{}, pathaddr.StructuralKey{}, false
	}
	parsed, ok := pathaddr.StructuralKeyFromPathKey(candidate)
	if !ok {
		return pathaddr.StructuralKey{}, pathaddr.StructuralKey{}, false
	}
	parsedPrefix, ok := pathaddr.StructuralKeyFromPathKey(prefix)
	if !ok {
		return pathaddr.StructuralKey{}, pathaddr.StructuralKey{}, false
	}
	return parsed, parsedPrefix, true
}
