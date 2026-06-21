package pathevidence

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type PathKeyDescendantInvalidationPrefixes struct {
	Descendants []pathdom.PathKey
	Subtrees    []pathdom.PathKey
}

// InvalidatePathKeySubtree removes finite path evidence at pathKey and any
// descendant key. It returns false when pathKey is not a recognized structural
// path-key spelling.
func (l Lane) InvalidatePathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (Lane, bool) {
	prefixes, ok := l.PathKeySubtreeInvalidationPrefixes(pathKey)
	if !ok {
		return l, false
	}
	match := func(candidate pathdom.PathKey) bool {
		return pathKeyInAnySubtree(candidate, prefixes)
	}
	prefixKeys := structuralPrefixKeys(ks, prefixes)
	return l.invalidatePathKeyEvidence(
		func(m map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool) {
			return deletePathKeySubtrees(ks, m, prefixKeys)
		},
		match,
		func(proof BranchProof) bool { return branchProofMatchesPath(proof, match) },
	)
}

// invalidatePathKeyEvidence drops refinement and static-member entries via
// deleteFromMap and branch proofs and presence implications whose path-key
// matches, returning the updated lane (or the receiver unchanged). proofMatch
// decides branch-proof removal separately so a length fact such as an
// index-in-range proof can clear with different scope than value-identity proofs.
func (l Lane) invalidatePathKeyEvidence(
	deleteFromMap func(map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool),
	match func(candidate pathdom.PathKey) bool,
	proofMatch func(proof BranchProof) bool,
) (Lane, bool) {
	refinements, changed := deleteFromMap(l.refinements)
	staticMembers, staticChanged := deleteFromMap(l.staticMembers)
	proofs, proofChanged := deleteBranchProofsWhere(l.proofs, proofMatch)
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
func (l Lane) InvalidatePathKeyDescendants(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (Lane, bool) {
	prefixes, ok := l.PathKeyDescendantInvalidationPrefixes(pathKey)
	if !ok {
		return l, false
	}
	match := func(candidate pathdom.PathKey) bool {
		return pathKeyInAnyDescendantInvalidation(candidate, prefixes)
	}
	descendantKeys := structuralPrefixKeys(ks, prefixes.Descendants)
	subtreeKeys := structuralPrefixKeys(ks, prefixes.Subtrees)
	return l.invalidatePathKeyEvidence(
		func(m map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool) {
			return deletePathKeyDescendantPrefixes(ks, m, descendantKeys, subtreeKeys)
		},
		match,
		func(proof BranchProof) bool {
			if branchProofMatchesPath(proof, match) {
				return true
			}
			// An index-in-range proof asserts index <= len(array); a write into the
			// array, including the container itself being invalidated, can change its
			// length. Unlike value-identity proofs, it must clear when the array (or
			// index) is the invalidation root, not only a strict descendant, matching
			// the non-strict length-floor invalidation.
			if proof.Kind == BranchProofIndexInRange {
				return pathKeyInDescendantInvalidationOrRoot(proof.Path, prefixes) ||
					pathKeyInDescendantInvalidationOrRoot(proof.Other, prefixes)
			}
			return false
		},
	)
}

// pathKeyInDescendantInvalidationOrRoot reports whether candidate is the
// invalidation root or any descendant of it, the non-strict scope a
// length-dependent fact must clear under.
func pathKeyInDescendantInvalidationOrRoot(candidate pathdom.PathKey, prefixes PathKeyDescendantInvalidationPrefixes) bool {
	for _, prefix := range prefixes.Descendants {
		if pathaddr.PathKeyHasPrefix(candidate, prefix) {
			return true
		}
	}
	for _, prefix := range prefixes.Subtrees {
		if pathaddr.PathKeyHasPrefix(candidate, prefix) {
			return true
		}
	}
	return false
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
	if rebased, ok := rebaseAcyclicAliasPathKey(prefix, from, to); ok {
		addPathKeyToQueue(rebased, seen, queue)
	}
	if pathaddr.PathKeyHasPrefix(from, prefix) {
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
	if rebased, ok := rebaseAcyclicAliasPathKey(prefix, from, to); ok {
		addDesc(rebased)
	}
	if pathaddr.PathKeyHasStrictPrefix(from, prefix) {
		addSubtree(to)
	}
}

func rebaseAcyclicAliasPathKey(pathKey, from, to pathdom.PathKey) (pathdom.PathKey, bool) {
	if cyclicDescendantExpansion(pathKey, from, to) {
		return "", false
	}
	return pathaddr.RebasePathKey(pathKey, from, to)
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

func pathKeyInAnySubtree(candidate pathdom.PathKey, prefixes []pathdom.PathKey) bool {
	for _, prefix := range prefixes {
		if pathaddr.PathKeyHasPrefix(candidate, prefix) {
			return true
		}
	}
	return false
}

func pathKeyInAnyDescendantInvalidation(candidate pathdom.PathKey, prefixes PathKeyDescendantInvalidationPrefixes) bool {
	for _, prefix := range prefixes.Descendants {
		if pathaddr.PathKeyHasStrictPrefix(candidate, prefix) {
			return true
		}
	}
	for _, prefix := range prefixes.Subtrees {
		if pathaddr.PathKeyHasPrefix(candidate, prefix) {
			return true
		}
	}
	return false
}

// structuralPrefixKeys interns the recognized structural path-key spellings in
// prefixes into comparable keyspace keys, dropping spellings that are not
// point-local value-lane keys.
func structuralPrefixKeys(ks *keyspace.KeySpace, prefixes []pathdom.PathKey) []keyspace.Key {
	if len(prefixes) == 0 {
		return nil
	}
	out := make([]keyspace.Key, 0, len(prefixes))
	for _, prefix := range prefixes {
		if key, ok := ks.FromPathKey(prefix); ok {
			out = append(out, key)
		}
	}
	return out
}

func deletePathKeySubtrees(
	ks *keyspace.KeySpace,
	in map[keyspace.Key]product.Value,
	prefixKeys []keyspace.Key,
) (map[keyspace.Key]product.Value, bool) {
	return deleteMatchingPathKeys(in, func(candidate keyspace.Key) bool {
		for _, prefix := range prefixKeys {
			if ks.HasPrefix(candidate, prefix) {
				return true
			}
		}
		return false
	})
}

func deletePathKeyDescendantPrefixes(
	ks *keyspace.KeySpace,
	in map[keyspace.Key]product.Value,
	descendantKeys []keyspace.Key,
	subtreeKeys []keyspace.Key,
) (map[keyspace.Key]product.Value, bool) {
	return deleteMatchingPathKeys(in, func(candidate keyspace.Key) bool {
		for _, prefix := range descendantKeys {
			if ks.HasStrictPrefix(candidate, prefix) {
				return true
			}
		}
		for _, prefix := range subtreeKeys {
			if ks.HasPrefix(candidate, prefix) {
				return true
			}
		}
		return false
	})
}

func deleteMatchingPathKeys(
	in map[keyspace.Key]product.Value,
	match func(keyspace.Key) bool,
) (map[keyspace.Key]product.Value, bool) {
	if len(in) == 0 {
		return in, false
	}
	out := make(map[keyspace.Key]product.Value, len(in))
	changed := false
	for pathKey, value := range in {
		if match(pathKey) {
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
