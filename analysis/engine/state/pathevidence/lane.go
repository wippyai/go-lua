package pathevidence

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Lane owns point-local path evidence whose invalidation semantics are coupled:
// path refinements, path static-member must facts, and branch proofs.
type Lane struct {
	refinements                    map[pathdom.PathKey]product.Value
	staticMembers                  map[pathdom.PathKey]product.Value
	proofs                         map[BranchProof]struct{}
	pathPresenceImplications       map[PathPresenceImplication]struct{}
	refinementsBottom              bool
	staticMembersBottom            bool
	proofsBottom                   bool
	pathPresenceImplicationsBottom bool
}

// Clone returns an independent copy of the lane's finite evidence.
func (l Lane) Clone() Lane {
	return Lane{
		refinements:                    clonePathValueMap(l.refinements),
		staticMembers:                  clonePathValueMap(l.staticMembers),
		proofs:                         cloneBranchProofSet(l.proofs),
		pathPresenceImplications:       clonePathPresenceImplicationSet(l.pathPresenceImplications),
		refinementsBottom:              l.refinementsBottom,
		staticMembersBottom:            l.staticMembersBottom,
		proofsBottom:                   l.proofsBottom,
		pathPresenceImplicationsBottom: l.pathPresenceImplicationsBottom,
	}
}

// Reachable clears must-lane bottom markers while preserving finite evidence.
func (l Lane) Reachable() Lane {
	l.refinementsBottom = false
	l.staticMembersBottom = false
	l.proofsBottom = false
	l.pathPresenceImplicationsBottom = false
	return l
}

// RefinementsBottom reports whether the path-refinement must sublane is bottom.
func (l Lane) RefinementsBottom() bool {
	return l.refinementsBottom
}

// StaticMembersBottom reports whether the static-member must sublane is bottom.
func (l Lane) StaticMembersBottom() bool {
	return l.staticMembersBottom
}

// ProofsBottom reports whether the branch-proof must sublane is bottom.
func (l Lane) ProofsBottom() bool {
	return l.proofsBottom
}

// PathPresenceImplicationsBottom reports whether the path-presence implication
// must sublane is bottom.
func (l Lane) PathPresenceImplicationsBottom() bool {
	return l.pathPresenceImplicationsBottom
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
