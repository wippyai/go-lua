package state

import (
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
