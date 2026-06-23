package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// PathRefinementsSnapshot returns finite must path refinements. Bottom is
// explicit; Top means the reachable must lane contains no finite refinements
// and callers must not manufacture finite facts from it.
func (s State) PathRefinementsSnapshot(ks *keyspace.KeySpace) pathevidence.PathRefinementsSnapshot {
	return s.pathEvidence.PathRefinementsSnapshot(ks)
}

// PathStaticMembersSnapshot returns finite must-static-member facts. Bottom is
// explicit; Top means the reachable must lane contains no finite facts.
func (s State) PathStaticMembersSnapshot(ks *keyspace.KeySpace) pathevidence.PathStaticMembersSnapshot {
	return s.pathEvidence.PathStaticMembersSnapshot(ks)
}

// BranchProofsSnapshot returns finite must branch proofs in stable order.
// Bottom is explicit; Top means the reachable must lane contains no proofs.
func (s State) BranchProofsSnapshot(ks *keyspace.KeySpace) pathevidence.BranchProofsSnapshot {
	return s.pathEvidence.BranchProofsSnapshot(ks)
}

// PathPresenceImplicationsSnapshot returns finite must path-presence
// implications in stable order. Bottom is explicit; Top means the reachable
// must lane contains no implications.
func (s State) PathPresenceImplicationsSnapshot(ks *keyspace.KeySpace) pathevidence.PathPresenceImplicationsSnapshot {
	return s.pathEvidence.PathPresenceImplicationsSnapshot(ks)
}

// ReadPathKey reads a point-local path refinement key. Missing keys read as
// product.Bottom(reg).
func (s State) ReadPathKey(reg *axis.Registry, ks *keyspace.KeySpace, pathKey pathdom.PathKey) product.Value {
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return product.Bottom(reg)
	}
	return s.ReadLocalPathKey(reg, localKey)
}

// ReadLocalPathKey reads an already-interned point-local path refinement key.
// Missing keys read as product.Bottom(reg).
func (s State) ReadLocalPathKey(reg *axis.Registry, pathKey keyspace.Key) product.Value {
	return s.pathEvidence.ReadPathKey(reg, pathKey)
}

// WritePathKey returns a state with pathKey updated. Writing
// product.Bottom(reg) removes the finite entry.
func (s State) WritePathKey(reg *axis.Registry, ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) State {
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return s
	}
	return s.WriteLocalPathKey(reg, localKey, value)
}

// WriteLocalPathKey returns a state with an already-interned point-local path
// refinement updated. Writing product.Bottom(reg) removes the finite entry.
func (s State) WriteLocalPathKey(reg *axis.Registry, pathKey keyspace.Key, value product.Value) State {
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
func (s State) UpdatePathKey(reg *axis.Registry, ks *keyspace.KeySpace, pathKey pathdom.PathKey, fn func(product.Value) product.Value) State {
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return s
	}
	return s.UpdateLocalPathKey(reg, localKey, fn)
}

// UpdateLocalPathKey reads an already-interned point-local path refinement key,
// applies fn, and writes the transformed value.
func (s State) UpdateLocalPathKey(reg *axis.Registry, pathKey keyspace.Key, fn func(product.Value) product.Value) State {
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
func (s State) InvalidatePathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (State, bool) {
	pathEvidence, ok := s.pathEvidence.InvalidatePathKeySubtree(ks, pathKey)
	if !ok {
		return s, false
	}
	prefixes, _ := s.pathEvidence.PathKeySubtreeInvalidationPrefixes(ks, pathKey)
	lenFloors, lenFloorChanged := s.lenFloors.clearPathKeySubtrees(ks, prefixes)
	out := s
	out.pathEvidence = pathEvidence
	if lenFloorChanged {
		out.lenFloors = lenFloors
	}
	return out, true
}

// InvalidatePathKeyDescendants removes finite path refinements below pathKey
// while preserving the exact pathKey refinement. It returns false when pathKey
// is not a recognized structural path-key spelling.
func (s State) InvalidatePathKeyDescendants(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (State, bool) {
	pathEvidence, ok := s.pathEvidence.InvalidatePathKeyDescendants(ks, pathKey)
	if !ok {
		return s, false
	}
	prefixes, _ := s.pathEvidence.PathKeyDescendantInvalidationPrefixes(ks, pathKey)
	lenFloors, lenFloorChanged := s.lenFloors.clearPathKeyDescendantMutation(ks, prefixes)
	out := s
	out.pathEvidence = pathEvidence
	if lenFloorChanged {
		out.lenFloors = lenFloors
	}
	return out, true
}

func (s State) PathKeyDescendantInvalidationPrefixes(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (pathevidence.PathKeyDescendantInvalidationPrefixes, bool) {
	return s.pathEvidence.PathKeyDescendantInvalidationPrefixes(ks, pathKey)
}

func (s State) ReadPathStaticMember(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (product.Value, bool) {
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return product.Value{}, false
	}
	return s.pathEvidence.ReadPathStaticMember(localKey)
}

func (s State) WritePathStaticMember(ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) State {
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return s
	}
	pathEvidence, reachable := s.pathEvidence.WritePathStaticMember(localKey, value)
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

// HasIndexInRangeProof is the legacy path-key adapter for in-range branch
// proofs. New boundary code that already resolved visibility should call
// HasIndexInRangeProofForStateKeys.
func (s State) HasIndexInRangeProof(ks *keyspace.KeySpace, indexKey, arrayKey pathdom.PathKey) bool {
	indexStateKey, ok := pathaddr.StateKeyFromPathKey(indexKey)
	if !ok {
		return false
	}
	arrayStateKey, ok := pathaddr.StateKeyFromPathKey(arrayKey)
	if !ok {
		return false
	}
	return s.HasIndexInRangeProofForStateKeys(ks, indexStateKey, arrayStateKey)
}

// HasIndexInRangeProofForStateKeys reports whether the state carries a
// must-proof that value(indexKey) is within len(arrayKey). Prefer this at
// module boundaries that already resolved paths to typed state keys.
func (s State) HasIndexInRangeProofForStateKeys(ks *keyspace.KeySpace, indexKey, arrayKey pathaddr.StateKey) bool {
	if indexKey == "" || arrayKey == "" {
		return false
	}
	index, ok := ks.InternStateKey(indexKey)
	if !ok {
		return false
	}
	array, ok := ks.InternStateKey(arrayKey)
	if !ok {
		return false
	}
	return s.HasBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofIndexInRange,
		Path:  index,
		Other: array,
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

func (s State) EquivalentPathKeys(ks *keyspace.KeySpace, pathKey pathdom.PathKey) []pathdom.PathKey {
	return s.pathEvidence.EquivalentPathKeys(ks, pathKey)
}

// RekeyPathEvidence re-interns the path-evidence value lane keys from one
// keyspace into another so a state built under one analysis's keyspace can be
// consumed as an entry state under another's. It is a no-op when from == to.
func (s State) RekeyPathEvidence(from, to *keyspace.KeySpace) State {
	rekeyed := s.pathEvidence.RekeyValueLanes(from, to)
	out := s
	out.pathEvidence = rekeyed
	out.numFloors = s.numFloors.rekey(from, to)
	out.lenFloors = s.lenFloors.rekey(from, to)
	out.heapTableIdentity = s.heapTableIdentity.rekey(from, to)
	return out
}
