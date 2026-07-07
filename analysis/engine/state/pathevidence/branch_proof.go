package pathevidence

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type BranchProofKind uint8

const (
	BranchProofPathPresence BranchProofKind = iota + 1
	BranchProofPathEqual
	BranchProofPathNotEqual
	BranchProofIndexInRange
)

type BranchProof struct {
	Kind     BranchProofKind
	Path     keyspace.Key
	Presence presence.Value
	Other    keyspace.Key
}

// AddBranchProof records a must fact that survived onto this control-flow edge.
func (l Lane) AddBranchProof(proof BranchProof) (Lane, bool) {
	if proof.Kind == 0 {
		return l, false
	}
	if !l.proofsBottom {
		if _, ok := l.proofs[proof]; ok {
			return l, false
		}
	}
	proofs := cloneBranchProofSet(l.proofs)
	if proofs == nil {
		proofs = make(map[BranchProof]struct{}, 1)
	}
	proofs[proof] = struct{}{}
	out := l.Reachable()
	out.proofs = proofs
	return out, true
}

func (l Lane) HasBranchProof(proof BranchProof) bool {
	if l.proofsBottom {
		return false
	}
	_, ok := l.proofs[proof]
	return ok
}

func (l Lane) HasBranchProofKind(kind BranchProofKind) bool {
	if l.proofsBottom || kind == 0 {
		return false
	}
	for proof := range l.proofs {
		if proof.Kind == kind {
			return true
		}
	}
	return false
}

func (l Lane) EquivalentPathKeys(ks *keyspace.KeySpace, pathKey pathdom.PathKey) []pathdom.PathKey {
	if pathKey == "" || l.proofsBottom || len(l.proofs) == 0 {
		return nil
	}
	start, ok := ks.FromStateKey(pathKey)
	if !ok {
		return nil
	}
	seen := map[pathdom.PathKey]struct{}{pathKey: {}}
	queue := []keyspace.Key{start}
	segmentLimit := equivalentPathExpansionSegmentLimit(ks, start, l.proofs)
	var out []pathdom.PathKey
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for proof := range l.proofs {
			if proof.Kind != BranchProofPathEqual {
				continue
			}
			for _, next := range equivalentPathKeysForProof(ks, current, proof) {
				if exceedsEquivalentPathSegmentLimit(ks, next, segmentLimit) {
					continue
				}
				spelling := ks.Format(next)
				if _, ok := seen[spelling]; ok {
					continue
				}
				seen[spelling] = struct{}{}
				out = append(out, spelling)
				queue = append(queue, next)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equivalentPathExpansionSegmentLimit(
	ks *keyspace.KeySpace,
	start keyspace.Key,
	proofs map[BranchProof]struct{},
) int {
	limit, ok := ks.SegmentLen(start)
	if !ok {
		return 0
	}
	for proof := range proofs {
		if proof.Kind != BranchProofPathEqual {
			continue
		}
		limit += positiveSegmentDelta(ks, proof.Path, proof.Other)
		limit += positiveSegmentDelta(ks, proof.Other, proof.Path)
	}
	return limit
}

func positiveSegmentDelta(ks *keyspace.KeySpace, from, to keyspace.Key) int {
	fromLen, fromOK := ks.SegmentLen(from)
	toLen, toOK := ks.SegmentLen(to)
	if !fromOK || !toOK || toLen <= fromLen {
		return 0
	}
	return toLen - fromLen
}

func exceedsEquivalentPathSegmentLimit(ks *keyspace.KeySpace, key keyspace.Key, limit int) bool {
	segments, ok := ks.SegmentLen(key)
	return !ok || segments > limit
}

// EquivalentRootKeys returns root-symbol keys proven equal to pathKey by exact
// equality endpoints. It is intentionally narrower than EquivalentPathKeys:
// root-value refinement only consumes root aliases, so following descendant
// rebases and formatting every intermediate path creates large allocation
// spikes while producing values the caller immediately discards.
func (l Lane) EquivalentRootKeys(ks *keyspace.KeySpace, pathKey pathdom.PathKey) []keyspace.Key {
	if pathKey == "" || l.proofsBottom || len(l.proofs) == 0 {
		return nil
	}
	start, ok := ks.FromStateKey(pathKey)
	if !ok {
		return nil
	}
	seen := map[keyspace.Key]struct{}{start: {}}
	queue := []keyspace.Key{start}
	var out []keyspace.Key
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for proof := range l.proofs {
			if proof.Kind != BranchProofPathEqual {
				continue
			}
			next, ok := exactEquivalentEndpoint(ks, current, proof)
			if !ok {
				continue
			}
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			if next.Segs == 0 {
				out = append(out, next)
			}
			queue = append(queue, next)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return ks.Format(out[i]) < ks.Format(out[j])
	})
	return out
}

func equivalentPathKeysForProof(ks *keyspace.KeySpace, pathKey keyspace.Key, proof BranchProof) []keyspace.Key {
	var out []keyspace.Key
	if rebased, ok := rebaseEquivalentPathKey(ks, pathKey, proof.Path, proof.Other); ok {
		out = append(out, rebased)
	}
	if rebased, ok := rebaseEquivalentPathKey(ks, pathKey, proof.Other, proof.Path); ok {
		out = append(out, rebased)
	}
	return out
}

func exactEquivalentEndpoint(ks *keyspace.KeySpace, current keyspace.Key, proof BranchProof) (keyspace.Key, bool) {
	if structuralKeysEquivalent(ks, current, proof.Path) {
		return proof.Other, true
	}
	if structuralKeysEquivalent(ks, current, proof.Other) {
		return proof.Path, true
	}
	return keyspace.Key{}, false
}

func structuralKeysEquivalent(ks *keyspace.KeySpace, a, b keyspace.Key) bool {
	return ks.HasPrefix(a, b) && ks.HasPrefix(b, a)
}

func rebaseEquivalentPathKey(ks *keyspace.KeySpace, pathKey, from, to keyspace.Key) (keyspace.Key, bool) {
	if cyclicDescendantExpansion(ks, pathKey, from, to) {
		return keyspace.Key{}, false
	}
	rebased, ok := ks.Rebase(pathKey, from, to)
	if !ok || !ks.HasPrefix(rebased, to) {
		return keyspace.Key{}, false
	}
	return rebased, true
}

func cyclicDescendantExpansion(ks *keyspace.KeySpace, pathKey, from, to keyspace.Key) bool {
	return ks.HasStrictPrefix(to, from) && ks.HasPrefix(pathKey, to)
}

func cloneBranchProofSet(in map[BranchProof]struct{}) map[BranchProof]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[BranchProof]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func deleteBranchProofsWhere(
	in map[BranchProof]struct{},
	matches func(BranchProof) bool,
) (map[BranchProof]struct{}, bool) {
	return mapedit.DeleteMatching(in, func(proof BranchProof, _ struct{}) bool {
		return matches(proof)
	})
}

func branchProofMatchesPath(proof BranchProof, matches func(keyspace.Key) bool) bool {
	if matches == nil {
		return false
	}
	if matches(proof.Path) {
		return true
	}
	switch proof.Kind {
	case BranchProofPathEqual, BranchProofPathNotEqual, BranchProofIndexInRange:
		return matches(proof.Other)
	default:
		return false
	}
}

func branchProofsFromSet(ks *keyspace.KeySpace, in map[BranchProof]struct{}) []BranchProof {
	if len(in) == 0 {
		return nil
	}
	ordered := make([]orderedBranchProof, 0, len(in))
	for proof := range in {
		ordered = append(ordered, orderedBranchProof{
			proof:    proof,
			path:     ks.Format(proof.Path),
			other:    ks.Format(proof.Other),
			presence: proof.Presence.String(),
		})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].less(ordered[j])
	})
	out := make([]BranchProof, len(ordered))
	for i, proof := range ordered {
		out[i] = proof.proof
	}
	return out
}

type orderedBranchProof struct {
	proof    BranchProof
	path     pathdom.PathKey
	other    pathdom.PathKey
	presence string
}

func (a orderedBranchProof) less(b orderedBranchProof) bool {
	if a.proof.Kind != b.proof.Kind {
		return a.proof.Kind < b.proof.Kind
	}
	if a.path != b.path {
		return a.path < b.path
	}
	if a.other != b.other {
		return a.other < b.other
	}
	return a.presence < b.presence
}
