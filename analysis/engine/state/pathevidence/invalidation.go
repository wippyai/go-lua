package pathevidence

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type PathKeyDescendantInvalidationPrefixes struct {
	Descendants []pathdom.PathKey
	Subtrees    []pathdom.PathKey
}

// InvalidatePathKeySubtree removes finite path evidence at pathKey and any
// descendant key. It returns false when pathKey is not a recognized structural
// path-key spelling.
func (l Lane) InvalidatePathKeySubtree(pathKey pathdom.PathKey) (Lane, bool) {
	prefixes, ok := l.PathKeySubtreeInvalidationPrefixes(pathKey)
	if !ok {
		return l, false
	}
	return l.invalidatePathKeyEvidence(
		func(m map[pathaddr.LocalKey]product.Value) (map[pathaddr.LocalKey]product.Value, bool) {
			return deletePathKeySubtrees(m, prefixes)
		},
		func(candidate pathdom.PathKey) bool {
			return pathKeyInAnySubtree(candidate, prefixes)
		},
	)
}

// invalidatePathKeyEvidence drops refinement and static-member entries via
// deleteFromMap and branch proofs and presence implications whose path-key
// matches, returning the updated lane (or the receiver unchanged).
func (l Lane) invalidatePathKeyEvidence(
	deleteFromMap func(map[pathaddr.LocalKey]product.Value) (map[pathaddr.LocalKey]product.Value, bool),
	match func(candidate pathdom.PathKey) bool,
) (Lane, bool) {
	refinements, changed := deleteFromMap(l.refinements)
	staticMembers, staticChanged := deleteFromMap(l.staticMembers)
	proofs, proofChanged := deleteBranchProofsMatching(l.proofs, match)
	implications, implicationChanged := deletePathPresenceImplicationsMatching(l.pathPresenceImplications, match)
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
	prefixes, ok := l.PathKeyDescendantInvalidationPrefixes(pathKey)
	if !ok {
		return l, false
	}
	return l.invalidatePathKeyEvidence(
		func(m map[pathaddr.LocalKey]product.Value) (map[pathaddr.LocalKey]product.Value, bool) {
			return deletePathKeyDescendantPrefixes(m, prefixes)
		},
		func(candidate pathdom.PathKey) bool {
			return pathKeyInAnyDescendantInvalidation(candidate, prefixes)
		},
	)
}

func (l Lane) PathKeySubtreeInvalidationPrefixes(pathKey pathdom.PathKey) ([]pathdom.PathKey, bool) {
	if _, ok := pathaddr.StructuralKeyFromPathKey(pathKey); !ok {
		return nil, false
	}
	return expandSubtreeInvalidationPrefixes([]pathdom.PathKey{pathKey}, l.proofs), true
}

func (l Lane) PathKeyDescendantInvalidationPrefixes(pathKey pathdom.PathKey) (PathKeyDescendantInvalidationPrefixes, bool) {
	if _, ok := pathaddr.StructuralKeyFromPathKey(pathKey); !ok {
		return PathKeyDescendantInvalidationPrefixes{}, false
	}
	return expandDescendantInvalidationPrefixes(pathKey, l.proofs), true
}

func expandSubtreeInvalidationPrefixes(seeds []pathdom.PathKey, proofs map[BranchProof]struct{}) []pathdom.PathKey {
	seen := make(map[pathdom.PathKey]struct{}, len(seeds))
	queue := make([]pathdom.PathKey, 0, len(seeds))
	for _, seed := range seeds {
		if seed == "" {
			continue
		}
		if _, ok := seen[seed]; ok {
			continue
		}
		seen[seed] = struct{}{}
		queue = append(queue, seed)
	}
	for len(queue) != 0 {
		prefix := queue[0]
		queue = queue[1:]
		for proof := range proofs {
			if proof.Kind != BranchProofPathEqual {
				continue
			}
			addSubtreeAliases(prefix, proof.Path, proof.Other, seen, &queue)
			addSubtreeAliases(prefix, proof.Other, proof.Path, seen, &queue)
		}
	}
	return sortedPathKeySet(seen)
}

func expandDescendantInvalidationPrefixes(pathKey pathdom.PathKey, proofs map[BranchProof]struct{}) PathKeyDescendantInvalidationPrefixes {
	descSeen := map[pathdom.PathKey]struct{}{pathKey: {}}
	descQueue := []pathdom.PathKey{pathKey}
	subtreeSeen := map[pathdom.PathKey]struct{}{}
	var subtreeQueue []pathdom.PathKey

	addDesc := func(pathKey pathdom.PathKey) {
		if pathKey == "" {
			return
		}
		if _, ok := descSeen[pathKey]; ok {
			return
		}
		descSeen[pathKey] = struct{}{}
		descQueue = append(descQueue, pathKey)
	}
	addSubtree := func(pathKey pathdom.PathKey) {
		if pathKey == "" {
			return
		}
		if _, ok := subtreeSeen[pathKey]; ok {
			return
		}
		subtreeSeen[pathKey] = struct{}{}
		subtreeQueue = append(subtreeQueue, pathKey)
	}

	for len(descQueue) != 0 || len(subtreeQueue) != 0 {
		for len(descQueue) != 0 {
			prefix := descQueue[0]
			descQueue = descQueue[1:]
			for proof := range proofs {
				if proof.Kind != BranchProofPathEqual {
					continue
				}
				addDescendantAliases(prefix, proof.Path, proof.Other, addDesc, addSubtree)
				addDescendantAliases(prefix, proof.Other, proof.Path, addDesc, addSubtree)
			}
		}
		for len(subtreeQueue) != 0 {
			prefix := subtreeQueue[0]
			subtreeQueue = subtreeQueue[1:]
			for proof := range proofs {
				if proof.Kind != BranchProofPathEqual {
					continue
				}
				addSubtreeAliases(prefix, proof.Path, proof.Other, subtreeSeen, &subtreeQueue)
				addSubtreeAliases(prefix, proof.Other, proof.Path, subtreeSeen, &subtreeQueue)
			}
		}
	}
	return PathKeyDescendantInvalidationPrefixes{
		Descendants: sortedPathKeySet(descSeen),
		Subtrees:    sortedPathKeySet(subtreeSeen),
	}
}

func addSubtreeAliases(
	prefix pathdom.PathKey,
	from pathdom.PathKey,
	to pathdom.PathKey,
	seen map[pathdom.PathKey]struct{},
	queue *[]pathdom.PathKey,
) {
	if rebased, ok := pathaddr.RebasePathKey(prefix, from, to); ok {
		addPathKeyToQueue(rebased, seen, queue)
	}
	if pathKeyInSubtree(from, prefix) {
		addPathKeyToQueue(to, seen, queue)
	}
}

func addDescendantAliases(
	prefix pathdom.PathKey,
	from pathdom.PathKey,
	to pathdom.PathKey,
	addDesc func(pathdom.PathKey),
	addSubtree func(pathdom.PathKey),
) {
	if rebased, ok := pathaddr.RebasePathKey(prefix, from, to); ok {
		addDesc(rebased)
	}
	if pathKeyInDescendants(from, prefix) {
		addSubtree(to)
	}
}

func addPathKeyToQueue(pathKey pathdom.PathKey, seen map[pathdom.PathKey]struct{}, queue *[]pathdom.PathKey) {
	if pathKey == "" {
		return
	}
	if _, ok := seen[pathKey]; ok {
		return
	}
	seen[pathKey] = struct{}{}
	*queue = append(*queue, pathKey)
}

func sortedPathKeySet(in map[pathdom.PathKey]struct{}) []pathdom.PathKey {
	if len(in) == 0 {
		return nil
	}
	out := make([]pathdom.PathKey, 0, len(in))
	for pathKey := range in {
		out = append(out, pathKey)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func pathKeyInSubtree(candidate pathdom.PathKey, prefix pathdom.PathKey) bool {
	parsed, parsedPrefix, ok := parsePathKeyPair(candidate, prefix)
	return ok && parsed.HasPrefix(parsedPrefix)
}

func pathKeyInDescendants(candidate pathdom.PathKey, prefix pathdom.PathKey) bool {
	parsed, parsedPrefix, ok := parsePathKeyPair(candidate, prefix)
	return ok && parsed.HasStrictPrefix(parsedPrefix)
}

func pathKeyInAnySubtree(candidate pathdom.PathKey, prefixes []pathdom.PathKey) bool {
	for _, prefix := range prefixes {
		if pathKeyInSubtree(candidate, prefix) {
			return true
		}
	}
	return false
}

func pathKeyInAnyDescendantInvalidation(candidate pathdom.PathKey, prefixes PathKeyDescendantInvalidationPrefixes) bool {
	for _, prefix := range prefixes.Descendants {
		if pathKeyInDescendants(candidate, prefix) {
			return true
		}
	}
	for _, prefix := range prefixes.Subtrees {
		if pathKeyInSubtree(candidate, prefix) {
			return true
		}
	}
	return false
}

func deletePathKeySubtrees(
	in map[pathaddr.LocalKey]product.Value,
	prefixes []pathdom.PathKey,
) (map[pathaddr.LocalKey]product.Value, bool) {
	out, changed := deleteMatchingPathKeys(in, func(candidate pathaddr.StructuralKey) bool {
		for _, prefix := range prefixes {
			parsedPrefix, ok := pathaddr.StructuralKeyFromPathKey(prefix)
			if ok && candidate.HasPrefix(parsedPrefix) {
				return true
			}
		}
		return false
	})
	return out, changed
}

func deletePathKeyDescendantPrefixes(
	in map[pathaddr.LocalKey]product.Value,
	prefixes PathKeyDescendantInvalidationPrefixes,
) (map[pathaddr.LocalKey]product.Value, bool) {
	out, changed := deleteMatchingPathKeys(in, func(candidate pathaddr.StructuralKey) bool {
		for _, prefix := range prefixes.Descendants {
			parsedPrefix, ok := pathaddr.StructuralKeyFromPathKey(prefix)
			if ok && candidate.HasStrictPrefix(parsedPrefix) {
				return true
			}
		}
		for _, prefix := range prefixes.Subtrees {
			parsedPrefix, ok := pathaddr.StructuralKeyFromPathKey(prefix)
			if ok && candidate.HasPrefix(parsedPrefix) {
				return true
			}
		}
		return false
	})
	return out, changed
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
