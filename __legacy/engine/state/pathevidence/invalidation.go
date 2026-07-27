package pathevidence

import (
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/internal/mapedit"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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

// InvalidateStableSymbolPreservingImplications atomically invalidates stable
// evidence rooted at sym while retaining implications independently proven
// valid by preserve. The implication must-set is traversed and rebuilt once.
func (l Lane) InvalidateStableSymbolPreservingImplications(sym symbol.ID, preserve func(PathPresenceImplication) bool) Lane {
	return l.invalidateStableSymbol(sym, preserve, false)
}

// InvalidateStableSymbolPreservingAllImplications invalidates non-implication
// evidence for an idempotent root write. Because no implication proposition
// changes, the persistent must-set is retained without traversal or copying.
func (l Lane) InvalidateStableSymbolPreservingAllImplications(sym symbol.ID) Lane {
	return l.invalidateStableSymbol(sym, nil, true)
}

func (l Lane) invalidateStableSymbol(sym symbol.ID, preserve func(PathPresenceImplication) bool, preserveAll bool) Lane {
	if sym == 0 {
		return l
	}
	match := func(candidate keyspace.Key) bool {
		return stableKeyBelongsToSymbol(candidate, sym)
	}
	return l.invalidatePathKeyEvidence(
		func(m map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool) {
			return deleteMatchingPathKeys(m, match)
		},
		match,
		func(proof BranchProof) bool { return branchProofMatchesPath(proof, match) },
		preserve,
		preserveAll,
	)
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
	return l.invalidatePathKeyEvidence(
		func(m map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool) {
			return deletePathKeySubtrees(ks, m, prefixKeys)
		},
		match,
		func(proof BranchProof) bool { return branchProofMatchesPath(proof, match) },
		nil,
		false,
	)
}

// InvalidatePathKeySubtreePrefixesChanged is the factor-native form. Subtree
// invalidation only deletes finite facts, so cardinality and Bottom markers
// decide semantic change without a whole-lane lattice comparison.
func (l Lane) InvalidatePathKeySubtreePrefixesChanged(ks *keyspace.KeySpace, prefixes []pathdom.PathKey) (Lane, bool) {
	next := l.InvalidatePathKeySubtreePrefixes(ks, prefixes)
	changed := l.refinementsBottom != next.refinementsBottom || len(l.refinements) != len(next.refinements) ||
		l.staticMembersBottom != next.staticMembersBottom || len(l.staticMembers) != len(next.staticMembers) ||
		l.proofsBottom != next.proofsBottom || len(l.proofs) != len(next.proofs) ||
		l.pathPresenceImplicationsBottom != next.pathPresenceImplicationsBottom || len(l.pathPresenceImplications) != len(next.pathPresenceImplications)
	return next, changed
}

// invalidatePathKeyEvidence drops refinement and static-member entries via
// deleteFromMap and branch proofs and presence implications whose path-key
// matches. proofMatch decides branch-proof removal separately so a length fact
// such as an index-in-range proof can clear with different scope than
// value-identity proofs.
func (l Lane) invalidatePathKeyEvidence(
	deleteFromMap func(map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool),
	match func(candidate keyspace.Key) bool,
	proofMatch func(proof BranchProof) bool,
	preserveImplication func(PathPresenceImplication) bool,
	preserveAllImplications bool,
) Lane {
	refinements, changed := deleteFromMap(l.refinements)
	staticMembers, staticChanged := deleteFromMap(l.staticMembers)
	proofs, proofChanged := deleteBranchProofsWhere(l.proofs, proofMatch)
	implications, implicationChanged := l.pathPresenceImplications, false
	if !preserveAllImplications {
		if preserveImplication == nil {
			implications, implicationChanged = deletePathPresenceImplicationsMatching(l.pathPresenceImplications, match)
		} else {
			implications, implicationChanged = deletePathPresenceImplicationsMatchingExcept(l.pathPresenceImplications, match, preserveImplication)
		}
	}
	if !changed && !staticChanged && !proofChanged && !implicationChanged {
		return l
	}
	out := l
	out.refinements = refinements
	out.staticMembers = staticMembers
	out.proofs = proofs
	out.pathPresenceImplications = implications
	return out
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
				return pathKeyInDescendantInvalidationOrRoot(ks, proof.Path, descendantKeys, subtreeKeys) ||
					pathKeyInDescendantInvalidationOrRoot(ks, proof.Other, descendantKeys, subtreeKeys)
			}
			return false
		},
		nil,
		false,
	)
}

// InvalidatePathKeyDescendantPrefixesChanged is the exact mutation form used
// by registered factor executors.  The operation only deletes finite facts;
// cardinality and Bottom-marker changes therefore decide semantic change
// without a whole-lane equality pass.
func (l Lane) InvalidatePathKeyDescendantPrefixesChanged(ks *keyspace.KeySpace, prefixes PathKeyDescendantInvalidationPrefixes) (Lane, bool) {
	next := l.InvalidatePathKeyDescendantPrefixes(ks, prefixes)
	changed := l.refinementsBottom != next.refinementsBottom || len(l.refinements) != len(next.refinements) ||
		l.staticMembersBottom != next.staticMembersBottom || len(l.staticMembers) != len(next.staticMembers) ||
		l.proofsBottom != next.proofsBottom || len(l.proofs) != len(next.proofs) ||
		l.pathPresenceImplicationsBottom != next.pathPresenceImplicationsBottom || len(l.pathPresenceImplications) != len(next.pathPresenceImplications)
	return next, changed
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
	for _, seed := range seeds {
		seedKey, ok := ks.FromStateKey(seed)
		if !ok {
			continue
		}
		for _, alias := range finiteEqualityInvalidationAliases(ks, seedKey, proofs, false) {
			seen[ks.Format(alias)] = struct{}{}
		}
	}
	return sortedPathKeySet(seen)
}

func expandDescendantInvalidationPrefixes(ks *keyspace.KeySpace, pathKey pathdom.PathKey, proofs map[BranchProof]struct{}) PathKeyDescendantInvalidationPrefixes {
	seed, ok := ks.FromStateKey(pathKey)
	if !ok {
		return PathKeyDescendantInvalidationPrefixes{}
	}
	descSeen := make(map[pathdom.PathKey]struct{})
	for _, alias := range finiteEqualityInvalidationAliases(ks, seed, proofs, true) {
		descSeen[ks.Format(alias)] = struct{}{}
	}
	subtreeSeen := make(map[pathdom.PathKey]struct{})
	for _, alias := range finiteEqualityInvalidationAliases(ks, seed, proofs, false) {
		if _, rootAlias := descSeen[ks.Format(alias)]; !rootAlias {
			subtreeSeen[ks.Format(alias)] = struct{}{}
		}
	}
	return PathKeyDescendantInvalidationPrefixes{
		Descendants: sortedPathKeySet(descSeen),
		Subtrees:    sortedPathKeySet(subtreeSeen),
	}
}

// finiteEqualityInvalidationAliases evaluates invalidation against the same
// finite observed equality carrier as proof closure. Queueing every freshly
// rebased prefix is unsound: cyclic equations manufacture endlessly longer
// prefixes and force KeySpace to intern each one.
//
// rootsOnly reports aliases exactly equal to seed. Otherwise it also reports
// aliases of observed strict descendants of those roots; callers use these as
// inclusive subtree prefixes.
func finiteEqualityInvalidationAliases(ks *keyspace.KeySpace, seed keyspace.Key, proofs map[BranchProof]struct{}, rootsOnly bool) []keyspace.Key {
	equalities := equalityProofsFromSet(proofs)
	if len(equalities) == 0 {
		return []keyspace.Key{seed}
	}
	lane, _ := (Lane{}).AddBranchProofs(equalities)
	congruence := newPathCongruence(ks, lane)
	carrier := finiteEqualityInvalidationBaseCarrier(seed, equalities)
	seedNormal, ok := congruence.normal(seed)
	if !ok {
		return []keyspace.Key{seed}
	}
	rootAliases := make([]keyspace.Key, 0)
	for _, candidate := range carrier {
		normal, valid := congruence.normal(candidate)
		if valid && pathCongruenceNormalsEqual(seedNormal, normal) {
			rootAliases = appendUniqueKey(rootAliases, candidate)
		}
	}
	carrier = finiteEqualityInvalidationCarrier(ks, seed, equalities, rootAliases)
	if rootsOnly {
		return rootAliases
	}
	anchors := make([]pathCongruenceNormal, 0)
	for _, candidate := range carrier {
		if !hasStrictPrefixIn(candidate, rootAliases, ks) {
			continue
		}
		normal, valid := congruence.normal(candidate)
		if valid {
			anchors = append(anchors, normal)
		}
	}
	out := append([]keyspace.Key(nil), rootAliases...)
	for _, candidate := range carrier {
		normal, valid := congruence.normal(candidate)
		if valid && anyCongruenceNormalEqual(normal, anchors) {
			out = appendUniqueKey(out, candidate)
		}
	}
	return out
}

func equalityProofsFromSet(proofs map[BranchProof]struct{}) []BranchProof {
	equalities := make([]BranchProof, 0)
	for proof := range proofs {
		if proof.Kind == BranchProofPathEqual {
			equalities = append(equalities, proof)
		}
	}
	return equalities
}

func finiteEqualityInvalidationBaseCarrier(seed keyspace.Key, equalities []BranchProof) []keyspace.Key {
	set := map[keyspace.Key]struct{}{seed: {}}
	endpoints := equalityClosureEndpoints(equalities)
	for _, endpoint := range endpoints {
		set[endpoint] = struct{}{}
	}
	out := make([]keyspace.Key, 0, len(set))
	for candidate := range set {
		out = append(out, candidate)
	}
	return out
}

func finiteEqualityInvalidationCarrier(ks *keyspace.KeySpace, seed keyspace.Key, equalities []BranchProof, rootAliases []keyspace.Key) []keyspace.Key {
	set := map[keyspace.Key]struct{}{}
	endpoints := equalityClosureEndpoints(equalities)
	observed := append([]keyspace.Key{seed}, endpoints...)
	for _, candidate := range observed {
		set[candidate] = struct{}{}
	}
	for _, source := range observed {
		if source != seed && containsKey(rootAliases, source) {
			continue
		}
		for _, from := range endpoints {
			suffix, ok := ks.ExactRemainderAfterPrefix(source, from)
			if !ok {
				continue
			}
			for _, to := range endpoints {
				if candidate, valid := appendPathSegments(ks, to, suffix); valid {
					set[candidate] = struct{}{}
				}
			}
		}
	}
	out := make([]keyspace.Key, 0, len(set))
	for candidate := range set {
		out = append(out, candidate)
	}
	return out
}

func containsKey(keys []keyspace.Key, candidate keyspace.Key) bool {
	for _, key := range keys {
		if key == candidate {
			return true
		}
	}
	return false
}

func hasStrictPrefixIn(candidate keyspace.Key, prefixes []keyspace.Key, ks *keyspace.KeySpace) bool {
	for _, prefix := range prefixes {
		if ks.HasStrictPrefix(candidate, prefix) {
			return true
		}
	}
	return false
}

func anyCongruenceNormalEqual(candidate pathCongruenceNormal, normals []pathCongruenceNormal) bool {
	for _, normal := range normals {
		if pathCongruenceNormalsEqual(candidate, normal) {
			return true
		}
	}
	return false
}

func appendUniqueKey(keys []keyspace.Key, candidate keyspace.Key) []keyspace.Key {
	for _, existing := range keys {
		if existing == candidate {
			return keys
		}
	}
	return append(keys, candidate)
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
		if ks.HasPathPrefix(candidate, prefix) {
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

// structuralPrefixKeys restores the exact typed structural identities emitted
// by KeySpace.FormatReadOnly. Prefix plans are shared by concrete resolver
// roots, formal roots, and other sealed State-key vocabularies; narrowing this
// round trip to resolver-only FromPathKey silently erases formal subtrees.
func structuralPrefixKeys(ks *keyspace.KeySpace, prefixes []pathdom.PathKey) []keyspace.Key {
	if len(prefixes) == 0 {
		return nil
	}
	out := make([]keyspace.Key, 0, len(prefixes))
	for _, prefix := range prefixes {
		if key, ok := ks.FromStateKey(prefix); ok {
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
		return pathKeyInAnyPrefix(ks, candidate, prefixKeys)
	})
}

func deletePathKeyDescendantPrefixes(
	ks *keyspace.KeySpace,
	in map[keyspace.Key]product.Value,
	descendantKeys []keyspace.Key,
	subtreeKeys []keyspace.Key,
) (map[keyspace.Key]product.Value, bool) {
	return deleteMatchingPathKeys(in, func(candidate keyspace.Key) bool {
		return pathKeyInAnyStrictPrefix(ks, candidate, descendantKeys) ||
			pathKeyInAnyPrefix(ks, candidate, subtreeKeys)
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
