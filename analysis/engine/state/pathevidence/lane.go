package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Lane owns point-local path evidence whose invalidation semantics are coupled:
// path refinements, path static-member must facts, and branch proofs. The
// refinement and static-member value sublanes key on the structural keyspace.Key
// (point-local resolver-symbol identities); the branch-proof and presence
// implication sublanes still carry path-key string spellings.
type Lane struct {
	refinements                    map[keyspace.KeyHandle]product.Value
	staticMembers                  map[keyspace.KeyHandle]product.Value
	proofs                         map[BranchProof]struct{}
	pathPresenceImplications       map[PathPresenceImplication]struct{}
	equalityRootMask               equalityRootMask
	refinementsBottom              bool
	staticMembersBottom            bool
	proofsBottom                   bool
	pathPresenceImplicationsBottom bool
}

type equalityRootMask [4]uint64

// Clone returns an independent copy of the lane's finite evidence.
func (l Lane) Clone() Lane {
	return Lane{
		refinements:                    cloneLocalValueHandleMap(l.refinements),
		staticMembers:                  cloneLocalValueHandleMap(l.staticMembers),
		proofs:                         cloneBranchProofSet(l.proofs),
		pathPresenceImplications:       clonePathPresenceImplicationSet(l.pathPresenceImplications),
		equalityRootMask:               l.equalityRootMask,
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

func cloneLocalValueMap(in map[keyspace.Key]product.Value) map[keyspace.Key]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[keyspace.Key]product.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneLocalValueHandleMap(in map[keyspace.KeyHandle]product.Value) map[keyspace.KeyHandle]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[keyspace.KeyHandle]product.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
