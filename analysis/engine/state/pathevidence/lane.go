package pathevidence

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Lane owns point-local path evidence whose invalidation semantics are coupled:
// path refinements, path static-member must facts, and branch proofs.
type Lane struct {
	refinements         map[pathdom.PathKey]product.Value
	staticMembers       map[pathdom.PathKey]product.Value
	proofs              map[BranchProof]struct{}
	refinementsTop      bool
	staticMembersBottom bool
	proofsBottom        bool
}

// Clone returns an independent copy of the lane's finite evidence.
func (l Lane) Clone() Lane {
	return Lane{
		refinements:         clonePathValueMap(l.refinements),
		staticMembers:       clonePathValueMap(l.staticMembers),
		proofs:              cloneBranchProofSet(l.proofs),
		refinementsTop:      l.refinementsTop,
		staticMembersBottom: l.staticMembersBottom,
		proofsBottom:        l.proofsBottom,
	}
}

// Reachable clears must-lane bottom markers while preserving finite evidence.
func (l Lane) Reachable() Lane {
	l.staticMembersBottom = false
	l.proofsBottom = false
	return l
}

// StaticMembersBottom reports whether the static-member must sublane is bottom.
func (l Lane) StaticMembersBottom() bool {
	return l.staticMembersBottom
}

// ProofsBottom reports whether the branch-proof must sublane is bottom.
func (l Lane) ProofsBottom() bool {
	return l.proofsBottom
}

func clonePathValueMap(in map[pathdom.PathKey]product.Value) map[pathdom.PathKey]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
