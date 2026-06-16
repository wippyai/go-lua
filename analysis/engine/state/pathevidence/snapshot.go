package pathevidence

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type PathRefinementsSnapshot struct {
	Bottom      bool
	Top         bool
	Refinements map[pathdom.PathKey]product.Value
}

// PathRefinementsSnapshot returns finite must path refinements. Bottom is
// explicit; Top means the reachable must lane contains no finite refinements.
func (l Lane) PathRefinementsSnapshot() PathRefinementsSnapshot {
	if l.refinementsBottom {
		return PathRefinementsSnapshot{Bottom: true}
	}
	refinements := snapshotLocalValueMap(l.refinements)
	return PathRefinementsSnapshot{
		Top:         len(refinements) == 0,
		Refinements: refinements,
	}
}

type PathStaticMembersSnapshot struct {
	Bottom  bool
	Top     bool
	Members map[pathdom.PathKey]product.Value
}

// PathStaticMembersSnapshot returns finite must-static-member facts.
func (l Lane) PathStaticMembersSnapshot() PathStaticMembersSnapshot {
	if l.staticMembersBottom {
		return PathStaticMembersSnapshot{Bottom: true}
	}
	members := snapshotLocalValueMap(l.staticMembers)
	return PathStaticMembersSnapshot{
		Top:     len(members) == 0,
		Members: members,
	}
}

func snapshotLocalValueMap(in map[pathaddr.LocalKey]product.Value) map[pathdom.PathKey]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
	for k, v := range in {
		out[k.PathKey()] = v
	}
	return out
}

type BranchProofsSnapshot struct {
	Bottom bool
	Top    bool
	Proofs []BranchProof
}

// BranchProofsSnapshot returns finite must branch proofs in stable order.
func (l Lane) BranchProofsSnapshot() BranchProofsSnapshot {
	if l.proofsBottom {
		return BranchProofsSnapshot{Bottom: true}
	}
	proofs := branchProofsFromSet(l.proofs)
	return BranchProofsSnapshot{
		Top:    len(proofs) == 0,
		Proofs: proofs,
	}
}

type PathPresenceImplicationsSnapshot struct {
	Bottom       bool
	Top          bool
	Implications []PathPresenceImplication
}

// PathPresenceImplicationsSnapshot returns finite must path-presence
// implications in stable order.
func (l Lane) PathPresenceImplicationsSnapshot() PathPresenceImplicationsSnapshot {
	if l.pathPresenceImplicationsBottom {
		return PathPresenceImplicationsSnapshot{Bottom: true}
	}
	implications := pathPresenceImplicationsFromSet(l.pathPresenceImplications)
	return PathPresenceImplicationsSnapshot{
		Top:          len(implications) == 0,
		Implications: implications,
	}
}
