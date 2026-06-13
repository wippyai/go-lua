package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type branchProofKey struct {
	kind     pathevidence.BranchProofKind
	path     pathdom.PathKey
	presence presence.Value
	other    pathdom.PathKey
}

func normalizeBranchProofs(in []callboundary.BranchProof) []callboundary.BranchProof {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[branchProofKey]callboundary.BranchProof, len(in))
	for _, proof := range in {
		if !proof.Path.IsPlaceholder() {
			continue
		}
		proof.Path = cloneSummaryPath(proof.Path)
		switch proof.Kind {
		case pathevidence.BranchProofPathPresence:
			if proof.Presence.IsBottom() || proof.Presence.IsTop() {
				continue
			}
			proof.Other = pathdom.Path{}
		case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual:
			if !proof.Other.IsPlaceholder() {
				continue
			}
			proof.Other = cloneSummaryPath(proof.Other)
			proof.Presence = presence.Bottom()
		default:
			continue
		}
		seen[branchProofKeyOf(proof)] = proof
	}
	return sortedBranchProofs(seen)
}

func cloneBranchProofs(in []callboundary.BranchProof) []callboundary.BranchProof {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.BranchProof, len(in))
	for i, proof := range in {
		proof.Path = cloneSummaryPath(proof.Path)
		proof.Other = cloneSummaryPath(proof.Other)
		out[i] = proof
	}
	return out
}

func branchProofsEqual(a, b []callboundary.BranchProof) bool {
	a = normalizeBranchProofs(a)
	b = normalizeBranchProofs(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if branchProofKeyOf(a[i]) != branchProofKeyOf(b[i]) {
			return false
		}
	}
	return true
}

func branchProofsLessOrEq(a, b []callboundary.BranchProof) bool {
	aSet := branchProofsSet(a)
	for _, proof := range normalizeBranchProofs(b) {
		if _, ok := aSet[branchProofKeyOf(proof)]; !ok {
			return false
		}
	}
	return true
}

func joinBranchProofs(a, b []callboundary.BranchProof) []callboundary.BranchProof {
	aSet := branchProofsSet(a)
	bSet := branchProofsSet(b)
	if len(aSet) == 0 || len(bSet) == 0 {
		return nil
	}
	out := make(map[branchProofKey]callboundary.BranchProof)
	for key, proof := range aSet {
		if _, ok := bSet[key]; ok {
			out[key] = proof
		}
	}
	return sortedBranchProofs(out)
}

func branchProofsSet(in []callboundary.BranchProof) map[branchProofKey]callboundary.BranchProof {
	out := normalizeBranchProofs(in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[branchProofKey]callboundary.BranchProof, len(out))
	for _, proof := range out {
		m[branchProofKeyOf(proof)] = proof
	}
	return m
}

func branchProofKeyOf(proof callboundary.BranchProof) branchProofKey {
	return branchProofKey{
		kind:     proof.Kind,
		path:     proof.Path.Key(),
		presence: proof.Presence,
		other:    proof.Other.Key(),
	}
}

func sortedBranchProofs(in map[branchProofKey]callboundary.BranchProof) []callboundary.BranchProof {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.BranchProof, 0, len(in))
	for _, proof := range in {
		out = append(out, proof)
	}
	sort.Slice(out, func(i, j int) bool {
		left := branchProofKeyOf(out[i])
		right := branchProofKeyOf(out[j])
		if left.kind != right.kind {
			return left.kind < right.kind
		}
		if left.path != right.path {
			return left.path < right.path
		}
		if left.presence != right.presence {
			return left.presence < right.presence
		}
		return left.other < right.other
	})
	return out
}
