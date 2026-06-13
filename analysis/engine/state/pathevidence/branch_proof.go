package pathevidence

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

type BranchProofKind uint8

const (
	BranchProofPathPresence BranchProofKind = iota + 1
	BranchProofPathEqual
	BranchProofPathNotEqual
)

type BranchProof struct {
	Kind     BranchProofKind
	Path     pathdom.PathKey
	Presence presence.Value
	Other    pathdom.PathKey
}

// AddBranchProof records a must fact that survived onto this control-flow edge.
func (l Lane) AddBranchProof(proof BranchProof) (Lane, bool) {
	if proof.Kind == 0 {
		return l, false
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

func (l Lane) EquivalentPathKeys(pathKey pathdom.PathKey) []pathdom.PathKey {
	if pathKey == "" || l.proofsBottom || len(l.proofs) == 0 {
		return nil
	}
	seen := map[pathdom.PathKey]struct{}{pathKey: {}}
	queue := []pathdom.PathKey{pathKey}
	var out []pathdom.PathKey
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for proof := range l.proofs {
			if proof.Kind != BranchProofPathEqual {
				continue
			}
			for _, next := range equivalentPathKeysForProof(current, proof) {
				if _, ok := seen[next]; ok {
					continue
				}
				seen[next] = struct{}{}
				out = append(out, next)
				queue = append(queue, next)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equivalentPathKeysForProof(pathKey pathdom.PathKey, proof BranchProof) []pathdom.PathKey {
	var out []pathdom.PathKey
	if rebased, ok := rebaseEquivalentPathKey(pathKey, proof.Path, proof.Other); ok {
		out = append(out, rebased)
	}
	if rebased, ok := rebaseEquivalentPathKey(pathKey, proof.Other, proof.Path); ok {
		out = append(out, rebased)
	}
	return out
}

func rebaseEquivalentPathKey(pathKey, from, to pathdom.PathKey) (pathdom.PathKey, bool) {
	rebased, ok := pathaddr.RebasePathKey(pathKey, from, to)
	if !ok || !pathKeyInSubtree(rebased, rebased) {
		return "", false
	}
	return rebased, true
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

func deleteBranchProofsMatching(
	in map[BranchProof]struct{},
	matches func(pathdom.PathKey) bool,
) (map[BranchProof]struct{}, bool) {
	if len(in) == 0 {
		return in, false
	}
	out := make(map[BranchProof]struct{}, len(in))
	changed := false
	for proof := range in {
		if branchProofMatchesPath(proof, matches) {
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

func branchProofMatchesPath(proof BranchProof, matches func(pathdom.PathKey) bool) bool {
	if matches == nil {
		return false
	}
	if matches(proof.Path) {
		return true
	}
	switch proof.Kind {
	case BranchProofPathEqual, BranchProofPathNotEqual:
		return matches(proof.Other)
	default:
		return false
	}
}

func branchProofsFromSet(in map[BranchProof]struct{}) []BranchProof {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchProof, 0, len(in))
	for proof := range in {
		out = append(out, proof)
	}
	sort.Slice(out, func(i, j int) bool {
		return branchProofLess(out[i], out[j])
	})
	return out
}

func branchProofLess(a, b BranchProof) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Other != b.Other {
		return a.Other < b.Other
	}
	return a.Presence.String() < b.Presence.String()
}
