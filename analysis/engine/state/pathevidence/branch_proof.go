package pathevidence

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
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
	var out []pathdom.PathKey
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for proof := range l.proofs {
			if proof.Kind != BranchProofPathEqual {
				continue
			}
			for _, next := range equivalentPathKeysForProof(ks, current, proof) {
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
	if len(in) == 0 {
		return in, false
	}
	out := make(map[BranchProof]struct{}, len(in))
	changed := false
	for proof := range in {
		if matches(proof) {
			changed = true
			continue
		}
		out[proof] = struct{}{}
	}
	if !changed {
		return in, false
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
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
	out := make([]BranchProof, 0, len(in))
	for proof := range in {
		out = append(out, proof)
	}
	sort.Slice(out, func(i, j int) bool {
		return branchProofLess(ks, out[i], out[j])
	})
	return out
}

func branchProofLess(ks *keyspace.KeySpace, a, b BranchProof) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Path != b.Path {
		return ks.Less(a.Path, b.Path)
	}
	if a.Other != b.Other {
		return ks.Less(a.Other, b.Other)
	}
	return a.Presence.String() < b.Presence.String()
}
