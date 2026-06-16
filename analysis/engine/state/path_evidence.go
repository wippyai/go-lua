package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type PathRefinementsSnapshot = pathevidence.PathRefinementsSnapshot

// PathRefinementsSnapshot returns finite must path refinements. Bottom is
// explicit; Top means the reachable must lane contains no finite refinements
// and callers must not manufacture finite facts from it.
func (s State) PathRefinementsSnapshot() PathRefinementsSnapshot {
	return s.pathEvidence.PathRefinementsSnapshot()
}

type PathStaticMembersSnapshot = pathevidence.PathStaticMembersSnapshot

// PathStaticMembersSnapshot returns finite must-static-member facts. Bottom is
// explicit; Top means the reachable must lane contains no finite facts.
func (s State) PathStaticMembersSnapshot() PathStaticMembersSnapshot {
	return s.pathEvidence.PathStaticMembersSnapshot()
}

type BranchProofsSnapshot = pathevidence.BranchProofsSnapshot

// BranchProofsSnapshot returns finite must branch proofs in stable order.
// Bottom is explicit; Top means the reachable must lane contains no proofs.
func (s State) BranchProofsSnapshot() BranchProofsSnapshot {
	return s.pathEvidence.BranchProofsSnapshot()
}

type PathPresenceImplicationsSnapshot = pathevidence.PathPresenceImplicationsSnapshot

// PathPresenceImplicationsSnapshot returns finite must path-presence
// implications in stable order. Bottom is explicit; Top means the reachable
// must lane contains no implications.
func (s State) PathPresenceImplicationsSnapshot() PathPresenceImplicationsSnapshot {
	return s.pathEvidence.PathPresenceImplicationsSnapshot()
}

// ReadPathKey reads a point-local path refinement key. Missing keys read as
// product.Bottom(reg).
func (s State) ReadPathKey(reg *axis.Registry, pathKey pathdom.PathKey) product.Value {
	return s.pathEvidence.ReadPathKey(reg, pathKey)
}

// WritePathKey returns a state with pathKey updated. Writing
// product.Bottom(reg) removes the finite entry.
func (s State) WritePathKey(reg *axis.Registry, pathKey pathdom.PathKey, value product.Value) State {
	pathEvidence, reachable := s.pathEvidence.WritePathKey(reg, pathKey, value)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}

// UpdatePathKey reads pathKey, applies fn, and writes the transformed value.
// Transforming a finite entry to product.Bottom(reg) removes it.
func (s State) UpdatePathKey(reg *axis.Registry, pathKey pathdom.PathKey, fn func(product.Value) product.Value) State {
	pathEvidence, reachable := s.pathEvidence.UpdatePathKey(reg, pathKey, fn)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}

// InvalidatePathKeySubtree removes finite path refinements at pathKey and any
// descendant key. It returns false when pathKey is not a recognized structural
// path-key spelling.
func (s State) InvalidatePathKeySubtree(pathKey pathdom.PathKey) (State, bool) {
	pathEvidence, ok := s.pathEvidence.InvalidatePathKeySubtree(pathKey)
	if !ok {
		return s, false
	}
	out := s
	out.pathEvidence = pathEvidence
	return out, true
}

// InvalidatePathKeyDescendants removes finite path refinements below pathKey
// while preserving the exact pathKey refinement. It returns false when pathKey
// is not a recognized structural path-key spelling.
func (s State) InvalidatePathKeyDescendants(pathKey pathdom.PathKey) (State, bool) {
	pathEvidence, ok := s.pathEvidence.InvalidatePathKeyDescendants(pathKey)
	if !ok {
		return s, false
	}
	out := s
	out.pathEvidence = pathEvidence
	return out, true
}

func (s State) ReadPathStaticMember(pathKey pathdom.PathKey) (product.Value, bool) {
	return s.pathEvidence.ReadPathStaticMember(pathKey)
}

func (s State) WritePathStaticMember(pathKey pathdom.PathKey, value product.Value) State {
	pathEvidence, reachable := s.pathEvidence.WritePathStaticMember(pathKey, value)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}

// AddBranchProof records a must fact that survived onto this control-flow
// edge. State joins keep only facts proven by all incoming predecessors, so
// these facts may be used for later aliasing/readback until path invalidation
// removes them.
func (s State) AddBranchProof(proof pathevidence.BranchProof) State {
	pathEvidence, reachable := s.pathEvidence.AddBranchProof(proof)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}

func (s State) HasBranchProof(proof pathevidence.BranchProof) bool {
	return s.pathEvidence.HasBranchProof(proof)
}

func (s State) AddIndexInRangeProof(indexKey, arrayKey pathdom.PathKey) State {
	if indexKey == "" || arrayKey == "" {
		return s
	}
	return s.AddBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofIndexInRange,
		Path:  indexKey,
		Other: arrayKey,
	})
}

func (s State) HasIndexInRangeProof(indexKey, arrayKey pathdom.PathKey) bool {
	if indexKey == "" || arrayKey == "" {
		return false
	}
	return s.HasBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofIndexInRange,
		Path:  indexKey,
		Other: arrayKey,
	})
}

// AddPathPresenceImplication records a must path-presence implication that
// remains valid until path invalidation removes either participating path.
func (s State) AddPathPresenceImplication(implication pathevidence.PathPresenceImplication) State {
	pathEvidence, reachable := s.pathEvidence.AddPathPresenceImplication(implication)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}

func (s State) HasPathPresenceImplication(implication pathevidence.PathPresenceImplication) bool {
	return s.pathEvidence.HasPathPresenceImplication(implication)
}

func (s State) EquivalentPathKeys(pathKey pathdom.PathKey) []pathdom.PathKey {
	return s.pathEvidence.EquivalentPathKeys(pathKey)
}
