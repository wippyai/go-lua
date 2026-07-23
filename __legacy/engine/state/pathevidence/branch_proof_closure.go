package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// CloseBranchProofsAcrossKnownEqualities computes the exact closure of the
// supplied proof carrier under its equality equations. The carrier is finite:
// an observed proof key may be translated only by retaining one of its
// observed suffixes and replacing its observed equality-endpoint prefix with
// another equality endpoint. Congruence decides which of those finite terms
// are equal.
//
// Equality equations can otherwise define an infinite ground class (for
// example a=b.child and b=a.child). Iterating Rebase over newly produced
// proofs would intern a.child.child... without an ascending-chain condition;
// it is neither a finite closure nor a sound representation of that class.
// The finite observed carrier is complete for every proof scalar this API can
// return, while arbitrary membership queries remain the responsibility of the
// congruence query API.
func CloseBranchProofsAcrossKnownEqualities(ks *keyspace.KeySpace, proofs []BranchProof) []BranchProof {
	if ks == nil || !ks.Valid() || len(proofs) == 0 {
		return append([]BranchProof(nil), proofs...)
	}
	set := make(map[BranchProof]struct{}, len(proofs))
	equalities := make([]BranchProof, 0)
	data := make([]BranchProof, 0)
	for _, proof := range proofs {
		if proof.Kind == 0 {
			continue
		}
		if _, duplicate := set[proof]; duplicate {
			continue
		}
		set[proof] = struct{}{}
		switch proof.Kind {
		case BranchProofPathEqual:
			equalities = append(equalities, proof)
		case BranchProofPathPresence, BranchProofIndexInRange:
			data = append(data, proof)
		}
	}
	if len(equalities) == 0 || len(data) == 0 {
		return branchProofsFromSet(ks, set)
	}

	lane, _ := (Lane{}).AddBranchProofs(equalities)
	congruence := newPathCongruence(ks, lane)
	for _, proof := range data {
		paths := closeBranchProofKeyCarrier(ks, congruence, proof.Path, equalities)
		others := []keyspace.Key{proof.Other}
		if proof.Kind == BranchProofIndexInRange && proof.Other != (keyspace.Key{}) {
			others = closeBranchProofKeyCarrier(ks, congruence, proof.Other, equalities)
		}
		for _, path := range paths {
			for _, other := range others {
				mirrored := proof
				mirrored.Path = path
				mirrored.Other = other
				set[mirrored] = struct{}{}
			}
		}
	}
	return branchProofsFromSet(ks, set)
}

// closeBranchProofKeyCarrier returns exactly the finite observed-shape terms
// congruent to source. It deliberately does not feed newly produced keys back
// into Rebase: doing so expands cyclic equality classes forever.
func closeBranchProofKeyCarrier(
	ks *keyspace.KeySpace,
	congruence *pathCongruence,
	source keyspace.Key,
	equalities []BranchProof,
) []keyspace.Key {
	if source.Kind == keyspace.KindInvalid {
		return []keyspace.Key{source}
	}
	candidates := map[keyspace.Key]struct{}{source: {}}
	endpoints := equalityClosureEndpoints(equalities)
	for _, from := range endpoints {
		suffix, ok := ks.ExactRemainderAfterPrefix(source, from)
		if !ok {
			continue
		}
		for _, to := range endpoints {
			if candidate, valid := appendPathSegments(ks, to, suffix); valid {
				candidates[candidate] = struct{}{}
			}
		}
	}
	sourceNormal, ok := congruence.normal(source)
	if !ok {
		return []keyspace.Key{source}
	}
	out := make([]keyspace.Key, 0, len(candidates))
	for candidate := range candidates {
		normal, valid := congruence.normal(candidate)
		if valid && pathCongruenceNormalsEqual(sourceNormal, normal) {
			out = append(out, candidate)
		}
	}
	return out
}

func equalityClosureEndpoints(equalities []BranchProof) []keyspace.Key {
	set := make(map[keyspace.Key]struct{}, len(equalities)*2)
	for _, equality := range equalities {
		set[equality.Path] = struct{}{}
		set[equality.Other] = struct{}{}
	}
	out := make([]keyspace.Key, 0, len(set))
	for endpoint := range set {
		out = append(out, endpoint)
	}
	return out
}

func appendPathSegments(ks *keyspace.KeySpace, key keyspace.Key, suffix []segment.Segment) (keyspace.Key, bool) {
	for _, part := range suffix {
		var ok bool
		key, ok = ks.AppendPathSegment(key, part)
		if !ok {
			return keyspace.Key{}, false
		}
	}
	return key, true
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
