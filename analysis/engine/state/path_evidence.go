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
	if !s.laneEnabled(lanePathEvidenceBit) {
		return pathevidence.PathRefinementsSnapshot{Bottom: true}
	}
	return s.pathEvidence.PathRefinementsSnapshot(ks)
}

// ForEachPathRefinement visits finite path-refinement facts without
// materializing a PathKey snapshot. Bottom and disabled lanes visit nothing.
func (s State) ForEachPathRefinement(fn func(keyspace.Key, product.Value) bool) {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return
	}
	s.pathEvidence.ForEachPathRefinement(fn)
}

// PathStaticMembersSnapshot returns finite must-static-member facts. Bottom is
// explicit; Top means the reachable must lane contains no finite facts.
func (s State) PathStaticMembersSnapshot(ks *keyspace.KeySpace) pathevidence.PathStaticMembersSnapshot {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return pathevidence.PathStaticMembersSnapshot{Bottom: true}
	}
	return s.pathEvidence.PathStaticMembersSnapshot(ks)
}

// ForEachPathStaticMember visits finite must-static-member facts without
// materializing a PathKey snapshot. Bottom and disabled lanes visit nothing.
func (s State) ForEachPathStaticMember(fn func(keyspace.Key, product.Value) bool) {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return
	}
	s.pathEvidence.ForEachPathStaticMember(fn)
}

// BranchProofsSnapshot returns finite must branch proofs in stable order.
// Bottom is explicit; Top means the reachable must lane contains no proofs.
func (s State) BranchProofsSnapshot(ks *keyspace.KeySpace) pathevidence.BranchProofsSnapshot {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return pathevidence.BranchProofsSnapshot{Bottom: true}
	}
	return s.pathEvidence.BranchProofsSnapshot(ks)
}

// PathPresenceImplicationsSnapshot returns finite must path-presence
// implications in stable order. Bottom is explicit; Top means the reachable
// must lane contains no implications.
func (s State) PathPresenceImplicationsSnapshot(ks *keyspace.KeySpace) pathevidence.PathPresenceImplicationsSnapshot {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return pathevidence.PathPresenceImplicationsSnapshot{Bottom: true}
	}
	return s.pathEvidence.PathPresenceImplicationsSnapshot(ks)
}

// ReadPathKey reads a point-local path refinement key. Missing keys read as
// product.Bottom(reg).
func (s State) ReadPathKey(reg *axis.Registry, ks *keyspace.KeySpace, pathKey pathdom.PathKey) product.Value {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return product.Bottom(reg)
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return product.Bottom(reg)
	}
	return s.ReadLocalPathKey(reg, localKey)
}

// ReadLocalPathKey reads an already-interned point-local path refinement key.
// Missing keys read as product.Bottom(reg).
func (s State) ReadLocalPathKey(reg *axis.Registry, pathKey keyspace.Key) product.Value {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return product.Bottom(reg)
	}
	return s.pathEvidence.ReadPathKey(reg, pathKey)
}

// WritePathKey returns a state with pathKey updated. Writing
// product.Bottom(reg) removes the finite entry.
func (s State) WritePathKey(reg *axis.Registry, ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) State {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return s
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return s
	}
	return s.WriteLocalPathKey(reg, localKey, value)
}

// WriteLocalPathKey returns a state with an already-interned point-local path
// refinement updated. Writing product.Bottom(reg) removes the finite entry.
func (s State) WriteLocalPathKey(reg *axis.Registry, pathKey keyspace.Key, value product.Value) State {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return s
	}
	pathEvidence, reachable := s.pathEvidence.WritePathKey(reg, pathKey, value)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}

// PathEvidenceEdit batches point-local path refinement and static-member writes
// against one State snapshot. It is equivalent to repeated WritePathKey and
// WritePathStaticMember calls, but clones each path-evidence value map at most
// once.
type PathEvidenceEdit struct {
	state   State
	enabled bool
	edit    pathevidence.Edit
}

// EditPathEvidence opens a path-evidence edit transaction. Call Done or DoneOn
// to publish the staged evidence.
func (s State) EditPathEvidence(reg *axis.Registry) PathEvidenceEdit {
	return PathEvidenceEdit{
		state:   s,
		enabled: s.laneEnabled(lanePathEvidenceBit),
		edit:    pathevidence.EditLane(reg, s.pathEvidence),
	}
}

// WritePathKey stages a path-refinement write after resolving the external
// path-key spelling through ks.
func (e *PathEvidenceEdit) WritePathKey(ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) bool {
	if e == nil || !e.enabled || ks == nil {
		return false
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return false
	}
	return e.WriteLocalPathKey(localKey, value)
}

// WriteLocalPathKey stages an already-interned path-refinement write.
func (e *PathEvidenceEdit) WriteLocalPathKey(pathKey keyspace.Key, value product.Value) bool {
	if e == nil || !e.enabled {
		return false
	}
	return e.edit.WritePathKey(pathKey, value)
}

// WritePathStaticMember stages a static-member write after resolving the
// external path-key spelling through ks.
func (e *PathEvidenceEdit) WritePathStaticMember(ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) bool {
	if e == nil || !e.enabled || ks == nil {
		return false
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return false
	}
	return e.WriteLocalPathStaticMember(localKey, value)
}

// WriteLocalPathStaticMember stages an already-interned static-member write.
func (e *PathEvidenceEdit) WriteLocalPathStaticMember(pathKey keyspace.Key, value product.Value) bool {
	if e == nil || !e.enabled {
		return false
	}
	return e.edit.WritePathStaticMember(pathKey, value)
}

// Done publishes staged path evidence onto the original edit state.
func (e *PathEvidenceEdit) Done() State {
	if e == nil {
		return State{}
	}
	return e.DoneOn(e.state)
}

// DoneOn publishes staged path evidence onto base. Callers must ensure no
// independent path-evidence writes were made to base while the edit was open.
func (e *PathEvidenceEdit) DoneOn(base State) State {
	if e == nil || !e.enabled {
		return base
	}
	pathEvidence, changed := e.edit.Done()
	if !changed {
		return base
	}
	out := base.reachable()
	out.pathEvidence = pathEvidence
	return out
}

// UpdatePathKey reads pathKey, applies fn, and writes the transformed value.
// Transforming a finite entry to product.Bottom(reg) removes it.
func (s State) UpdatePathKey(reg *axis.Registry, ks *keyspace.KeySpace, pathKey pathdom.PathKey, fn func(product.Value) product.Value) State {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return s
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return s
	}
	return s.UpdateLocalPathKey(reg, localKey, fn)
}

// UpdateLocalPathKey reads an already-interned point-local path refinement key,
// applies fn, and writes the transformed value.
func (s State) UpdateLocalPathKey(reg *axis.Registry, pathKey keyspace.Key, fn func(product.Value) product.Value) State {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return s
	}
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
	return s.invalidatePathKeySubtree(ks, pathKey, true)
}

func (s State) InvalidatePathKeySubtreePreservingDynamicValueKeyMemberships(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (State, bool) {
	return s.invalidatePathKeySubtree(ks, pathKey, false)
}

func (s State) invalidatePathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey, clearDynamicValueMemberships bool) (State, bool) {
	prefixes, ok := s.pathEvidence.PathKeySubtreeInvalidationPrefixes(ks, pathKey)
	if !ok {
		return s, false
	}
	pathEvidence := s.pathEvidence.InvalidatePathKeySubtreePrefixes(ks, prefixes)
	lenFloors, lenFloorChanged := s.lenFloors.clearPathKeySubtrees(ks, prefixes)
	out := s
	out.pathEvidence = pathEvidence
	out = out.ClearDynamicIndexFactsForPathKeySubtree(ks, pathKey)
	if clearDynamicValueMemberships {
		out = out.ClearKeyMembershipsForPathKeySubtree(ks, pathKey)
	} else {
		out = out.ClearPathKeyMembershipsForPathKeySubtree(ks, pathKey)
	}
	if lenFloorChanged {
		out.lenFloors = lenFloors
	}
	return out, true
}

// InvalidatePathKeyDescendants removes finite path refinements below pathKey
// while preserving the exact pathKey refinement. It returns false when pathKey
// is not a recognized structural path-key spelling.
func (s State) InvalidatePathKeyDescendants(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (State, bool) {
	prefixes, ok := s.pathEvidence.PathKeyDescendantInvalidationPrefixes(ks, pathKey)
	if !ok {
		return s, false
	}
	pathEvidence := s.pathEvidence.InvalidatePathKeyDescendantPrefixes(ks, prefixes)
	lenFloors, lenFloorChanged := s.lenFloors.clearPathKeyDescendantMutation(ks, prefixes)
	out := s
	out.pathEvidence = pathEvidence
	out = out.ClearDynamicIndexFactsForPathKeyDescendants(ks, pathKey)
	if lenFloorChanged {
		out.lenFloors = lenFloors
	}
	return out, true
}

func (s State) InvalidatePathKeyDescendantsPreservingDynamicValueKeyMemberships(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (State, bool) {
	return s.InvalidatePathKeyDescendants(ks, pathKey)
}

func (s State) PathKeyDescendantInvalidationPrefixes(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (pathevidence.PathKeyDescendantInvalidationPrefixes, bool) {
	return s.pathEvidence.PathKeyDescendantInvalidationPrefixes(ks, pathKey)
}

func (s State) ReadPathStaticMember(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (product.Value, bool) {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return product.Value{}, false
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return product.Value{}, false
	}
	return s.ReadLocalPathStaticMember(localKey)
}

// ReadLocalPathStaticMember reads an already-interned static-member evidence
// key. Prefer this at boundaries that already resolved the path structurally.
func (s State) ReadLocalPathStaticMember(pathKey keyspace.Key) (product.Value, bool) {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return product.Value{}, false
	}
	return s.pathEvidence.ReadPathStaticMember(pathKey)
}

func (s State) WritePathStaticMember(ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) State {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return s
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return s
	}
	return s.WriteLocalPathStaticMember(localKey, value)
}

// WriteLocalPathStaticMember writes an already-interned static-member evidence
// key. Prefer this at boundaries that already resolved the path structurally.
func (s State) WriteLocalPathStaticMember(pathKey keyspace.Key, value product.Value) State {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return s
	}
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
	if !s.laneEnabled(lanePathEvidenceBit) {
		return s
	}
	pathEvidence, reachable := s.pathEvidence.AddBranchProof(proof)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}

func (s State) HasBranchProof(proof pathevidence.BranchProof) bool {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return false
	}
	return s.pathEvidence.HasBranchProof(proof)
}

// HasIndexInRangeProof is the path-key compatibility adapter for in-range
// branch proofs. Boundary code that already resolved visibility should call
// HasIndexInRangeProofForStateKeys.
func (s State) HasIndexInRangeProof(ks *keyspace.KeySpace, indexKey, arrayKey pathdom.PathKey) bool {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return false
	}
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
	if !s.laneEnabled(lanePathEvidenceBit) {
		return false
	}
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
	if !s.laneEnabled(lanePathEvidenceBit) {
		return s
	}
	pathEvidence, reachable := s.pathEvidence.AddPathPresenceImplication(implication)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}

func (s State) HasPathPresenceImplication(implication pathevidence.PathPresenceImplication) bool {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return false
	}
	return s.pathEvidence.HasPathPresenceImplication(implication)
}

func (s State) EquivalentPathKeys(ks *keyspace.KeySpace, pathKey pathdom.PathKey) []pathdom.PathKey {
	if !s.laneEnabled(lanePathEvidenceBit) {
		return nil
	}
	return s.pathEvidence.EquivalentPathKeys(ks, pathKey)
}

// EquivalentRootKeys returns root-symbol aliases proven equal to stateKey
// without expanding descendant rebases. Use this for root-value refinement;
// callers that need descendant path aliases should use EquivalentPathKeys.
func (s State) EquivalentRootKeys(ks *keyspace.KeySpace, stateKey pathaddr.StateKey) []keyspace.Key {
	if !s.laneEnabled(lanePathEvidenceBit) || stateKey == "" {
		return nil
	}
	return s.pathEvidence.EquivalentRootKeys(ks, stateKey.PathKey())
}

// EquivalentStateKeys returns validated state-key aliases proven equivalent to
// stateKey. Prefer this over EquivalentPathKeys at semantic boundaries that
// already require the root-or-visible state-key grammar.
func (s State) EquivalentStateKeys(ks *keyspace.KeySpace, stateKey pathaddr.StateKey) []pathaddr.StateKey {
	if !s.laneEnabled(lanePathEvidenceBit) || stateKey == "" {
		return nil
	}
	pathKeys := s.pathEvidence.EquivalentPathKeys(ks, stateKey.PathKey())
	if len(pathKeys) == 0 {
		return nil
	}
	out := make([]pathaddr.StateKey, 0, len(pathKeys))
	for _, pathKey := range pathKeys {
		equivalent, ok := pathaddr.StateKeyFromPathKey(pathKey)
		if !ok {
			continue
		}
		out = append(out, equivalent)
	}
	return out
}

// RekeyPathEvidence re-interns the path-evidence value lane keys from one
// keyspace into another so a state built under one analysis's keyspace can be
// consumed as an entry state under another's. It is a no-op when from == to.
func (s State) RekeyPathEvidence(from, to *keyspace.KeySpace) State {
	out := s
	if s.laneEnabled(lanePathEvidenceBit) {
		out.pathEvidence = s.pathEvidence.RekeyValueLanes(from, to)
	}
	if s.laneEnabled(laneNumFloorsBit) {
		out.numFloors = s.numFloors.rekey(from, to)
	}
	if s.laneEnabled(laneLenFloorsBit) {
		out.lenFloors = s.lenFloors.rekey(from, to)
	}
	if s.laneEnabled(laneHeapTableIdentityBit) {
		out.heapTableIdentity = s.heapTableIdentity.rekey(from, to)
	}
	return out
}
