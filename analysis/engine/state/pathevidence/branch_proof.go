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
	return l.AddBranchProofs([]BranchProof{proof})
}

// AddBranchProofs records each valid must proof with a single copy-on-write
// update. It is equivalent to repeated AddBranchProof calls.
func (l Lane) AddBranchProofs(additions []BranchProof) (Lane, bool) {
	if len(additions) == 0 {
		return l, false
	}
	var proofs map[BranchProof]struct{}
	changed := false
	for _, proof := range additions {
		if proof.Kind == 0 {
			continue
		}
		if !l.proofsBottom {
			if _, ok := l.proofs[proof]; ok {
				continue
			}
		}
		if proofs == nil {
			proofs = cloneBranchProofSet(l.proofs)
			if proofs == nil {
				proofs = make(map[BranchProof]struct{}, len(additions))
			}
		}
		if _, ok := proofs[proof]; ok {
			continue
		}
		proofs[proof] = struct{}{}
		changed = true
	}
	if !changed {
		return l, false
	}
	out := l.Reachable()
	out.proofs = proofs
	for _, proof := range additions {
		out.equalityRootMask.merge(equalityProofRootMask(proof))
	}
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

// BranchProofPresence returns the unique path-presence proof for key. It
// reports false when no proof exists or when conflicting presence proofs make
// the result unusable.
func (l Lane) BranchProofPresence(key keyspace.Key) (presence.Value, bool) {
	if l.proofsBottom || key.Kind == keyspace.KindInvalid {
		return presence.Bottom(), false
	}
	var value presence.Value
	found := false
	for proof := range l.proofs {
		if proof.Kind != BranchProofPathPresence || proof.Path != key {
			continue
		}
		if found && !presence.Equal(value, proof.Presence) {
			return presence.Bottom(), false
		}
		value = proof.Presence
		found = true
	}
	return value, found
}

func (l Lane) EquivalentPathKeys(ks *keyspace.KeySpace, pathKey pathdom.PathKey) []pathdom.PathKey {
	if pathKey == "" || l.proofsBottom || len(l.proofs) == 0 {
		return nil
	}
	start, ok := ks.FromStateKey(pathKey)
	if !ok {
		return nil
	}
	keys := l.EquivalentKeyspaceKeys(ks, start)
	if len(keys) == 0 {
		return nil
	}
	out := make([]pathdom.PathKey, len(keys))
	for i, key := range keys {
		out[i] = ks.Format(key)
	}
	return out
}

// EquivalentKeyspaceKeys returns every key reachable from start through
// equality proofs, including safe descendant rebases. The result is in stable
// keyspace order and excludes start itself.
func (l Lane) EquivalentKeyspaceKeys(ks *keyspace.KeySpace, start keyspace.Key) []keyspace.Key {
	if ks == nil || start.Kind == keyspace.KindInvalid || l.proofsBottom || len(l.proofs) == 0 {
		return nil
	}
	if !l.equalityRootMask.empty() && !l.equalityRootMask.matches(start) {
		return nil
	}
	seen := map[keyspace.Key]struct{}{start: {}}
	queue := []keyspace.Key{start}
	segmentLimit := equivalentPathExpansionSegmentLimit(ks, start, l.proofs)
	var out []keyspace.Key
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for proof := range l.proofs {
			if proof.Kind != BranchProofPathEqual {
				continue
			}
			if next, ok := rebaseEquivalentPathKey(ks, current, proof.Path, proof.Other); ok &&
				!exceedsEquivalentPathSegmentLimit(ks, next, segmentLimit) {
				if _, seenAlready := seen[next]; !seenAlready {
					seen[next] = struct{}{}
					out = append(out, next)
					queue = append(queue, next)
				}
			}
			if next, ok := rebaseEquivalentPathKey(ks, current, proof.Other, proof.Path); ok &&
				!exceedsEquivalentPathSegmentLimit(ks, next, segmentLimit) {
				if _, seenAlready := seen[next]; !seenAlready {
					seen[next] = struct{}{}
					out = append(out, next)
					queue = append(queue, next)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return ks.Less(out[i], out[j]) })
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

func equalityProofRootMask(proof BranchProof) equalityRootMask {
	if proof.Kind != BranchProofPathEqual {
		return equalityRootMask{}
	}
	mask := equalityRootMaskForKey(proof.Path)
	mask.merge(equalityRootMaskForKey(proof.Other))
	return mask
}

func equalityRootMaskForKey(key keyspace.Key) equalityRootMask {
	root := uint64(key.Kind)<<56 | uint64(key.Sym)<<24 | uint64(key.Ver)
	root ^= uint64(key.Root) * 0x9e3779b97f4a7c15
	root ^= root >> 30
	root *= 0xbf58476d1ce4e5b9
	root ^= root >> 27
	var mask equalityRootMask
	bit := root & 255
	mask[bit>>6] = uint64(1) << (bit & 63)
	return mask
}

func (m *equalityRootMask) merge(other equalityRootMask) {
	for i := range m {
		m[i] |= other[i]
	}
}

func (m equalityRootMask) empty() bool {
	return m == equalityRootMask{}
}

func (m equalityRootMask) matches(key keyspace.Key) bool {
	needle := equalityRootMaskForKey(key)
	for i := range m {
		if m[i]&needle[i] != 0 {
			return true
		}
	}
	return false
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
