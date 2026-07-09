package pathevidence

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type PathKeyDescendantInvalidationPrefixes struct {
	Descendants []pathdom.PathKey
	Subtrees    []pathdom.PathKey
}

// InvalidatePathKeySubtree removes finite path evidence at pathKey and any
// descendant key. It returns false when pathKey is not a recognized structural
// path-key spelling.
func (l Lane) InvalidatePathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (Lane, bool) {
	prefixes, ok := l.PathKeySubtreeInvalidationPrefixes(ks, pathKey)
	if !ok {
		return l, false
	}
	return l.InvalidatePathKeySubtreePrefixes(ks, prefixes), true
}

// InvalidateStableSymbol removes path evidence whose root is a versionless or
// stable spelling of sym. Point-visible resolver-version evidence is invalidated
// by the caller's normal path-subtree invalidation; this cleanup covers
// root-level implications stored outside that versioned address space.
func (l Lane) InvalidateStableSymbol(sym symbol.ID) Lane {
	if sym == 0 {
		return l
	}
	match := func(candidate keyspace.Key) bool {
		return stableKeyBelongsToSymbol(candidate, sym)
	}
	out, _ := l.invalidatePathKeyEvidence(
		func(m map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool) {
			return deleteMatchingPathKeys(m, match)
		},
		match,
		func(proof BranchProof) bool { return branchProofMatchesPath(proof, match) },
	)
	return out
}

func stableKeyBelongsToSymbol(candidate keyspace.Key, sym symbol.ID) bool {
	switch candidate.Kind {
	case keyspace.KindUnversionedSym, keyspace.KindStableSym:
		return candidate.Sym == sym
	default:
		return false
	}
}

// InvalidatePathKeySubtreePrefixes removes finite path evidence for a
// precomputed subtree invalidation plan. Callers that need the same plan for
// coupled lanes should compute PathKeySubtreeInvalidationPrefixes once and pass
// it here instead of recomputing alias expansion.
func (l Lane) InvalidatePathKeySubtreePrefixes(ks *keyspace.KeySpace, prefixes []pathdom.PathKey) Lane {
	prefixKeys := structuralPrefixKeys(ks, prefixes)
	match := func(candidate keyspace.Key) bool {
		return pathKeyInAnyPrefix(ks, candidate, prefixKeys)
	}
	out, _ := l.invalidatePathKeyEvidence(
		func(m map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool) {
			return deletePathKeySubtrees(ks, m, prefixKeys)
		},
		match,
		func(proof BranchProof) bool { return branchProofMatchesPath(proof, match) },
	)
	return out
}

// invalidatePathKeyEvidence drops refinement and static-member entries via
// deleteFromMap and branch proofs and presence implications whose path-key
// matches, returning the updated lane (or the receiver unchanged). proofMatch
// decides branch-proof removal separately so a length fact such as an
// index-in-range proof can clear with different scope than value-identity proofs.
func (l Lane) invalidatePathKeyEvidence(
	deleteFromMap func(map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool),
	match func(candidate keyspace.Key) bool,
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
	prefixes, ok := l.PathKeyDescendantInvalidationPrefixes(ks, pathKey)
	if !ok {
		return l, false
	}
	return l.InvalidatePathKeyDescendantPrefixes(ks, prefixes), true
}

// InvalidatePathKeyDescendantPrefixes removes finite path evidence for a
// precomputed descendant invalidation plan. It is the plan-consuming companion
// to PathKeyDescendantInvalidationPrefixes.
func (l Lane) InvalidatePathKeyDescendantPrefixes(ks *keyspace.KeySpace, prefixes PathKeyDescendantInvalidationPrefixes) Lane {
	descendantKeys := structuralPrefixKeys(ks, prefixes.Descendants)
	subtreeKeys := structuralPrefixKeys(ks, prefixes.Subtrees)
	match := func(candidate keyspace.Key) bool {
		return pathKeyInAnyStrictPrefix(ks, candidate, descendantKeys) ||
			pathKeyInAnyPrefix(ks, candidate, subtreeKeys)
	}
	out, _ := l.invalidatePathKeyEvidence(
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
				return pathKeyInDescendantInvalidationOrRoot(ks, proof.Path, descendantKeys, subtreeKeys) ||
					pathKeyInDescendantInvalidationOrRoot(ks, proof.Other, descendantKeys, subtreeKeys)
			}
			return false
		},
	)
	return out
}

// pathKeyInDescendantInvalidationOrRoot reports whether candidate is the
// invalidation root or any descendant of it, the non-strict scope a
// length-dependent fact must clear under.
func pathKeyInDescendantInvalidationOrRoot(ks *keyspace.KeySpace, candidate keyspace.Key, descendantKeys, subtreeKeys []keyspace.Key) bool {
	return pathKeyInAnyPrefix(ks, candidate, descendantKeys) ||
		pathKeyInAnyPrefix(ks, candidate, subtreeKeys)
}

func (l Lane) PathKeySubtreeInvalidationPrefixes(ks *keyspace.KeySpace, pathKey pathdom.PathKey) ([]pathdom.PathKey, bool) {
	if _, ok := ks.FromStateKey(pathKey); !ok {
		return nil, false
	}
	return expandSubtreeInvalidationPrefixes(ks, []pathdom.PathKey{pathKey}, l.proofs), true
}

func (l Lane) PathKeyDescendantInvalidationPrefixes(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (PathKeyDescendantInvalidationPrefixes, bool) {
	if _, ok := ks.FromStateKey(pathKey); !ok {
		return PathKeyDescendantInvalidationPrefixes{}, false
	}
	return expandDescendantInvalidationPrefixes(ks, pathKey, l.proofs), true
}

func expandSubtreeInvalidationPrefixes(ks *keyspace.KeySpace, seeds []pathdom.PathKey, proofs map[BranchProof]struct{}) []pathdom.PathKey {
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
			addSubtreeAliases(ks, prefix, proof.Path, proof.Other, seen, &queue)
			addSubtreeAliases(ks, prefix, proof.Other, proof.Path, seen, &queue)
		}
	}
	return sortedPathKeySet(seen)
}

func expandDescendantInvalidationPrefixes(ks *keyspace.KeySpace, pathKey pathdom.PathKey, proofs map[BranchProof]struct{}) PathKeyDescendantInvalidationPrefixes {
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
				addDescendantAliases(ks, prefix, proof.Path, proof.Other, addDesc, addSubtree)
				addDescendantAliases(ks, prefix, proof.Other, proof.Path, addDesc, addSubtree)
			}
		}
		for len(subtreeQueue) != 0 {
			prefix := subtreeQueue[0]
			subtreeQueue = subtreeQueue[1:]
			for proof := range proofs {
				if proof.Kind != BranchProofPathEqual {
					continue
				}
				addSubtreeAliases(ks, prefix, proof.Path, proof.Other, subtreeSeen, &subtreeQueue)
				addSubtreeAliases(ks, prefix, proof.Other, proof.Path, subtreeSeen, &subtreeQueue)
			}
		}
	}
	return PathKeyDescendantInvalidationPrefixes{
		Descendants: sortedPathKeySet(descSeen),
		Subtrees:    sortedPathKeySet(subtreeSeen),
	}
}

func addSubtreeAliases(
	ks *keyspace.KeySpace,
	prefix pathdom.PathKey,
	from keyspace.Key,
	to keyspace.Key,
	seen map[pathdom.PathKey]struct{},
	queue *[]pathdom.PathKey,
) {
	prefixKey, ok := ks.FromStateKey(prefix)
	if !ok {
		return
	}
	if rebased, ok := rebaseAcyclicAliasPathKey(ks, prefixKey, from, to); ok {
		addPathKeyToQueue(ks.Format(rebased), seen, queue)
	}
	if ks.HasPrefix(from, prefixKey) {
		addPathKeyToQueue(ks.Format(to), seen, queue)
	}
}

func addDescendantAliases(
	ks *keyspace.KeySpace,
	prefix pathdom.PathKey,
	from keyspace.Key,
	to keyspace.Key,
	addDesc func(pathdom.PathKey),
	addSubtree func(pathdom.PathKey),
) {
	prefixKey, ok := ks.FromStateKey(prefix)
	if !ok {
		return
	}
	if rebased, ok := rebaseAcyclicAliasPathKey(ks, prefixKey, from, to); ok {
		addDesc(ks.Format(rebased))
	}
	if ks.HasStrictPrefix(from, prefixKey) {
		addSubtree(ks.Format(to))
	}
}

func rebaseAcyclicAliasPathKey(ks *keyspace.KeySpace, pathKey, from, to keyspace.Key) (keyspace.Key, bool) {
	if cyclicDescendantExpansion(ks, pathKey, from, to) {
		return keyspace.Key{}, false
	}
	return ks.Rebase(pathKey, from, to)
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

func pathKeyInAnyPrefix(ks *keyspace.KeySpace, candidate keyspace.Key, prefixes []keyspace.Key) bool {
	for _, prefix := range prefixes {
		if ks.HasPrefix(candidate, prefix) {
			return true
		}
	}
	return false
}

func pathKeyInAnyStrictPrefix(ks *keyspace.KeySpace, candidate keyspace.Key, prefixes []keyspace.Key) bool {
	for _, prefix := range prefixes {
		if ks.HasStrictPrefix(candidate, prefix) {
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
			if stable, ok := localStableCounterpart(ks, key); ok {
				out = append(out, stable)
			}
		}
	}
	return out
}

func localStableCounterpart(ks *keyspace.KeySpace, key keyspace.Key) (keyspace.Key, bool) {
	if ks == nil || key.Kind != keyspace.KindResolverSym || key.Sym == 0 {
		return keyspace.Key{}, false
	}
	segments, ok := ks.SegmentsView(key)
	if !ok {
		return keyspace.Key{}, false
	}
	return ks.FromStableSymbol(key.Sym, segments)
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
	return mapedit.DeleteMatching(in, func(pathKey keyspace.Key, _ product.Value) bool {
		return match(pathKey)
	})
}
