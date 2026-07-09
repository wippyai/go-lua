package pathevidence

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type PathRefinementsSnapshot struct {
	Bottom      bool
	Top         bool
	Refinements map[pathdom.PathKey]product.Value
}

// PathRefinementsSnapshot returns finite must path refinements. Bottom is
// explicit; Top means the reachable must lane contains no finite refinements.
func (l Lane) PathRefinementsSnapshot(ks *keyspace.KeySpace) PathRefinementsSnapshot {
	if l.refinementsBottom {
		return PathRefinementsSnapshot{Bottom: true}
	}
	refinements := snapshotLocalValueMap(ks, l.refinements)
	return PathRefinementsSnapshot{
		Top:         len(refinements) == 0,
		Refinements: refinements,
	}
}

// ForEachPathRefinement visits finite path-refinement facts in their native
// keyspace form, avoiding the snapshot PathKey materialization path.
func (l Lane) ForEachPathRefinement(fn func(keyspace.Key, product.Value) bool) {
	if l.refinementsBottom || len(l.refinements) == 0 || fn == nil {
		return
	}
	for key, value := range l.refinements {
		if !fn(key, value) {
			return
		}
	}
}

type PathStaticMembersSnapshot struct {
	Bottom  bool
	Top     bool
	Members map[pathdom.PathKey]product.Value
}

// PathStaticMembersSnapshot returns finite must-static-member facts.
func (l Lane) PathStaticMembersSnapshot(ks *keyspace.KeySpace) PathStaticMembersSnapshot {
	if l.staticMembersBottom {
		return PathStaticMembersSnapshot{Bottom: true}
	}
	members := snapshotLocalValueMap(ks, l.staticMembers)
	return PathStaticMembersSnapshot{
		Top:     len(members) == 0,
		Members: members,
	}
}

// ForEachPathStaticMember visits finite must-static-member facts in their
// native keyspace form, avoiding the snapshot PathKey materialization path.
func (l Lane) ForEachPathStaticMember(fn func(keyspace.Key, product.Value) bool) {
	if l.staticMembersBottom || len(l.staticMembers) == 0 || fn == nil {
		return
	}
	for key, value := range l.staticMembers {
		if !fn(key, value) {
			return
		}
	}
}

func snapshotLocalValueMap(ks *keyspace.KeySpace, in map[keyspace.Key]product.Value) map[pathdom.PathKey]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
	for k, v := range in {
		out[ks.Format(k)] = v
	}
	return out
}

type BranchProofsSnapshot struct {
	Bottom bool
	Top    bool
	Proofs []BranchProof
}

// BranchProofsSnapshot returns finite must branch proofs in stable order.
func (l Lane) BranchProofsSnapshot(ks *keyspace.KeySpace) BranchProofsSnapshot {
	if l.proofsBottom {
		return BranchProofsSnapshot{Bottom: true}
	}
	proofs := branchProofsFromSet(ks, l.proofs)
	return BranchProofsSnapshot{
		Top:    len(proofs) == 0,
		Proofs: proofs,
	}
}

// ForEachBranchProof visits finite must branch proofs in their native form,
// avoiding stable snapshot materialization. Bottom and empty lanes visit
// nothing.
func (l Lane) ForEachBranchProof(fn func(BranchProof) bool) {
	if l.proofsBottom || len(l.proofs) == 0 || fn == nil {
		return
	}
	for proof := range l.proofs {
		if !fn(proof) {
			return
		}
	}
}

type PathPresenceImplicationsSnapshot struct {
	Bottom       bool
	Top          bool
	Implications []PathPresenceImplication
}

// PathPresenceImplicationsSnapshot returns finite must path-presence
// implications in stable order.
func (l Lane) PathPresenceImplicationsSnapshot(ks *keyspace.KeySpace) PathPresenceImplicationsSnapshot {
	if l.pathPresenceImplicationsBottom {
		return PathPresenceImplicationsSnapshot{Bottom: true}
	}
	implications := pathPresenceImplicationsFromSet(ks, l.pathPresenceImplications)
	return PathPresenceImplicationsSnapshot{
		Top:          len(implications) == 0,
		Implications: implications,
	}
}
