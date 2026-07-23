package callboundary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type branchProofKey struct {
	kind     pathevidence.BranchProofKind
	path     pathdom.PathKey
	presence presence.Value
	other    pathdom.PathKey
}

// branchProofLane is the canonical must (intersection) lattice for branch
// proofs: each proof is canonicalized per kind (presence proofs clear Other,
// equality proofs clear Presence) and kept only when guaranteed on every path.
var branchProofLane = factset.Set[branchProofKey, BranchProof]{
	Key:       branchProofKeyOf,
	EqualFact: func(a, b BranchProof) bool { return branchProofKeyOf(a) == branchProofKeyOf(b) },
	Less:      branchProofLess,
	Admit:     AdmitBranchProof,
	CloneFact: func(p BranchProof) BranchProof {
		p.Path = p.Path.Clone()
		p.Other = p.Other.Clone()
		return p
	},
	Prefer:    func(kept, incoming BranchProof) bool { return true },
	Intersect: true,
}

func AdmitBranchProof(proof BranchProof) (BranchProof, bool) {
	if !proof.Path.IsPlaceholder() {
		return proof, false
	}
	switch proof.Kind {
	case pathevidence.BranchProofPathPresence:
		if proof.Presence.IsBottom() || proof.Presence.IsTop() {
			return proof, false
		}
		proof.Other = pathdom.Path{}
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
		if !proof.Other.IsPlaceholder() {
			return proof, false
		}
		proof.Presence = presence.Bottom()
	default:
		return proof, false
	}
	return proof, true
}

func branchProofKeyOf(proof BranchProof) branchProofKey {
	return branchProofKey{
		kind:     proof.Kind,
		path:     proof.Path.Key(),
		presence: proof.Presence,
		other:    proof.Other.Key(),
	}
}

func branchProofLess(a, b BranchProof) bool {
	left := branchProofKeyOf(a)
	right := branchProofKeyOf(b)
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
}
