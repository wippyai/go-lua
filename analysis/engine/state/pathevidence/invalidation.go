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
	refinements, changed, ok := deletePathKeySubtree(l.refinements, pathKey)
	if !ok {
		return l, false
	}
	staticMembers, staticChanged, _ := deletePathKeySubtree(l.staticMembers, pathKey)
	proofs, proofChanged := deleteBranchProofsMatching(l.proofs, func(candidate pathdom.PathKey) bool {
		return pathKeyInSubtree(candidate, pathKey)
	})
	implications, implicationChanged := deletePathPresenceImplicationsMatching(l.pathPresenceImplications, func(candidate pathdom.PathKey) bool {
		return pathKeyInSubtree(candidate, pathKey)
	})
	if !changed && !staticChanged && !proofChanged && !implicationChanged {
		return l, true
	}
	out := l
	out.refinements = refinements
	out.staticMembers = staticMembers
	out.proofs = proofs
	out.pathPresenceImplications = implications
	return out, true
}

// InvalidatePathKeyDescendants removes finite path evidence below pathKey while
// preserving exact pathKey evidence. It returns false when pathKey is not a
// recognized structural path-key spelling.
func (l Lane) InvalidatePathKeyDescendants(pathKey pathdom.PathKey) (Lane, bool) {
	refinements, changed, ok := deletePathKeyDescendants(l.refinements, pathKey)
	if !ok {
		return l, false
	}
	staticMembers, staticChanged, _ := deletePathKeyDescendants(l.staticMembers, pathKey)
	proofs, proofChanged := deleteBranchProofsMatching(l.proofs, func(candidate pathdom.PathKey) bool {
		return pathKeyInDescendants(candidate, pathKey)
	})
	implications, implicationChanged := deletePathPresenceImplicationsMatching(l.pathPresenceImplications, func(candidate pathdom.PathKey) bool {
		return pathKeyInDescendants(candidate, pathKey)
	})
	if !changed && !staticChanged && !proofChanged && !implicationChanged {
		return l, true
	}
	out := l
	out.refinements = refinements
	out.staticMembers = staticMembers
	out.proofs = proofs
	out.pathPresenceImplications = implications
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
	in map[pathaddr.LocalKey]product.Value,
	prefix pathdom.PathKey,
) (map[pathaddr.LocalKey]product.Value, bool, bool) {
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
	in map[pathaddr.LocalKey]product.Value,
	prefix pathdom.PathKey,
) (map[pathaddr.LocalKey]product.Value, bool, bool) {
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
	in map[pathaddr.LocalKey]product.Value,
	match func(pathaddr.StructuralKey) bool,
) (map[pathaddr.LocalKey]product.Value, bool) {
	if len(in) == 0 {
		return in, false
	}
	out := make(map[pathaddr.LocalKey]product.Value, len(in))
	changed := false
	for pathKey, value := range in {
		parsed, ok := pathaddr.StructuralKeyFromPathKey(pathKey.PathKey())
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
