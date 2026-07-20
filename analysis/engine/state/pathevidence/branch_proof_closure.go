package pathevidence

import "github.com/wippyai/go-lua/analysis/domain/path/keyspace"

// CloseBranchProofsAcrossKnownEqualities computes the exact finite closure of
// Presence and IndexInRange proofs under every known path equality. It is pure,
// deterministic and cap-free; both concrete execution and coordinate planning
// use this one family-owned structural law.
func CloseBranchProofsAcrossKnownEqualities(ks *keyspace.KeySpace, proofs []BranchProof) []BranchProof {
	if ks == nil || !ks.Valid() || len(proofs) == 0 {
		return append([]BranchProof(nil), proofs...)
	}
	set := make(map[BranchProof]struct{}, len(proofs))
	equalities := make([]BranchProof, 0)
	for _, proof := range proofs {
		if proof.Kind == 0 {
			continue
		}
		if _, duplicate := set[proof]; duplicate {
			continue
		}
		set[proof] = struct{}{}
		if proof.Kind == BranchProofPathEqual {
			equalities = append(equalities, proof)
		}
	}
	for changed := true; changed; {
		changed = false
		current := make([]BranchProof, 0, len(set))
		for proof := range set {
			current = append(current, proof)
		}
		for _, equality := range equalities {
			for _, proof := range current {
				for _, direction := range [][2]keyspace.Key{{equality.Path, equality.Other}, {equality.Other, equality.Path}} {
					mirrored, ok := mirrorBranchProofAcrossEquality(ks, proof, direction[0], direction[1])
					if !ok {
						continue
					}
					if _, exists := set[mirrored]; !exists {
						set[mirrored] = struct{}{}
						changed = true
					}
				}
			}
		}
	}
	return branchProofsFromSet(ks, set)
}

func mirrorBranchProofAcrossEquality(ks *keyspace.KeySpace, proof BranchProof, fromKey, toKey keyspace.Key) (BranchProof, bool) {
	rebasedPath, ok := rebaseBranchProofKey(ks, proof.Path, fromKey, toKey)
	if !ok {
		return BranchProof{}, false
	}
	mirrored := proof
	mirrored.Path = rebasedPath
	switch proof.Kind {
	case BranchProofPathPresence:
		return mirrored, true
	case BranchProofIndexInRange:
		if proof.Other != (keyspace.Key{}) {
			if rebasedOther, otherOK := rebaseBranchProofKey(ks, proof.Other, fromKey, toKey); otherOK {
				mirrored.Other = rebasedOther
			}
		}
		return mirrored, true
	default:
		return BranchProof{}, false
	}
}

func rebaseBranchProofKey(ks *keyspace.KeySpace, proofKey, fromKey, toKey keyspace.Key) (keyspace.Key, bool) {
	if !branchProofKeysMayShareRoot(proofKey, fromKey) || !ks.HasPrefix(proofKey, fromKey) {
		return keyspace.Key{}, false
	}
	if ks.HasStrictPrefix(toKey, fromKey) && ks.HasPrefix(proofKey, toKey) {
		return keyspace.Key{}, false
	}
	rebased, ok := ks.Rebase(proofKey, fromKey, toKey)
	if !ok || !ks.HasPrefix(rebased, toKey) || rebased == proofKey {
		return keyspace.Key{}, false
	}
	return rebased, true
}

func branchProofKeysMayShareRoot(key, prefix keyspace.Key) bool {
	if key.Kind != prefix.Kind {
		return false
	}
	switch key.Kind {
	case keyspace.KindResolverSym:
		return key.Sym == prefix.Sym && key.Ver == prefix.Ver
	case keyspace.KindStableSym:
		return key.Sym == prefix.Sym
	case keyspace.KindNamed, keyspace.KindPlaceholder, keyspace.KindRetSlot:
		return key.Root == prefix.Root
	default:
		return true
	}
}
