package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type PathRefinementsSnapshot struct {
	Top         bool
	Refinements map[pathdom.PathKey]product.Value
}

// PathRefinementsSnapshot returns finite path refinements unless the path lane
// is top. When Top is true, Refinements is empty and callers must not
// manufacture finite facts from it.
func (s State) PathRefinementsSnapshot() PathRefinementsSnapshot {
	snapshot := s.pathEvidence.PathRefinementsSnapshot()
	return PathRefinementsSnapshot{
		Top:         snapshot.Top,
		Refinements: snapshot.Refinements,
	}
}

type PathStaticMembersSnapshot struct {
	Bottom  bool
	Top     bool
	Members map[pathdom.PathKey]product.Value
}

// PathStaticMembersSnapshot returns finite must-static-member facts. Bottom is
// explicit; Top means the reachable must lane contains no finite facts.
func (s State) PathStaticMembersSnapshot() PathStaticMembersSnapshot {
	snapshot := s.pathEvidence.PathStaticMembersSnapshot()
	return PathStaticMembersSnapshot{
		Bottom:  snapshot.Bottom,
		Top:     snapshot.Top,
		Members: snapshot.Members,
	}
}

type BranchProofsSnapshot struct {
	Bottom bool
	Top    bool
	Proofs []pathevidence.BranchProof
}

// BranchProofsSnapshot returns finite must branch proofs in stable order.
// Bottom is explicit; Top means the reachable must lane contains no proofs.
func (s State) BranchProofsSnapshot() BranchProofsSnapshot {
	snapshot := s.pathEvidence.BranchProofsSnapshot()
	return BranchProofsSnapshot{
		Bottom: snapshot.Bottom,
		Top:    snapshot.Top,
		Proofs: snapshot.Proofs,
	}
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

func (s State) EquivalentPathKeys(pathKey pathdom.PathKey) []pathdom.PathKey {
	return s.pathEvidence.EquivalentPathKeys(pathKey)
}
