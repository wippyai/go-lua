package pathevidence

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type PathRefinementsSnapshot struct {
	Top         bool
	Refinements map[pathdom.PathKey]product.Value
}

// PathRefinementsSnapshot returns finite path refinements unless the path lane
// is top. When Top is true, Refinements is empty.
func (l Lane) PathRefinementsSnapshot() PathRefinementsSnapshot {
	if l.refinementsTop {
		return PathRefinementsSnapshot{Top: true}
	}
	return PathRefinementsSnapshot{Refinements: clonePathValueMap(l.refinements)}
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
	members := clonePathValueMap(l.staticMembers)
	return PathStaticMembersSnapshot{
		Top:     len(members) == 0,
		Members: members,
	}
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
