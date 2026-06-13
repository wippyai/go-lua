package state

import (
	"sort"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
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

// AddBranchProof records a must fact that survived onto this control-flow
// edge. State joins keep only facts proven by all incoming predecessors, so
// these facts may be used for later aliasing/readback until path invalidation
// removes them.
func (s State) AddBranchProof(proof BranchProof) State {
	if proof.Kind == 0 {
		return s
	}
	proofs := cloneBranchProofSet(s.branchProofs)
	if proofs == nil {
		proofs = make(map[BranchProof]struct{}, 1)
	}
	proofs[proof] = struct{}{}
	out := s.reachable()
	out.branchProofs = proofs
	return out
}

func (s State) HasBranchProof(proof BranchProof) bool {
	if s.branchProofsBottom {
		return false
	}
	_, ok := s.branchProofs[proof]
	return ok
}

func (s State) EquivalentPathKeys(pathKey pathdom.PathKey) []pathdom.PathKey {
	if pathKey == "" || s.branchProofsBottom || len(s.branchProofs) == 0 {
		return nil
	}
	seen := map[pathdom.PathKey]struct{}{pathKey: {}}
	queue := []pathdom.PathKey{pathKey}
	var out []pathdom.PathKey
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for proof := range s.branchProofs {
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
	if from == "" || to == "" || !pathKeyInSubtree(pathKey, from) {
		return "", false
	}
	suffix := strings.TrimPrefix(string(pathKey), string(from))
	rebased := pathdom.PathKey(string(to) + suffix)
	if rebased == pathKey || !pathKeyInSubtree(rebased, rebased) {
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
