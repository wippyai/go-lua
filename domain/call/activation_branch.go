// activation_branch.go is Call's owner surface for the activation branch set:
// the candidate body routes one mounted call site may instantiate.
//
// The set is the whole global body table, because a Call value may name any
// admitted body - Bodies says so in as many words. It hangs off a mounted call
// row rather than being addressed by an occurrence of its own, so it is a
// nested member set of the mounted-call directory, and its rows are enumerated
// rather than read: a branch carries no Call fact any judgment consumes.

package call

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

const (
	// activationTargetFrame and activationEndpointFrame are the two semantic
	// axes one body route is issued under. They are frames of the body's own
	// portable role, so two bodies with one role are one route and a single
	// body's two endpoints stay distinct.
	activationTargetFrame   = uint64(1)
	activationEndpointFrame = uint64(2)
	// activationApplicationFrame is the frame the application a trigger is one
	// occurrence of is issued under.
	activationApplicationFrame = uint64(1)
)

// ActivationBranchCount is the census of the branch set one mounted call row
// carries. Every admitted body is a candidate route: which of them a trigger
// actually names is the Call value's answer at solve time, not this table's.
func (row CallCoordinate) ActivationBranchCount() int {
	if !row.Valid() {
		return 0
	}
	return row.owner.Bodies().Count()
}

// ActivationBranchAt addresses one branch of that set by its ordinal, which is
// the address the set's rows are reached by under their parent.
func (row CallCoordinate) ActivationBranchAt(ordinal int) (Body, bool) {
	if !row.Valid() {
		return Body{}, false
	}
	return row.owner.Bodies().At(ordinal)
}

// ActivationApplication is the semantic identity of the application this
// mounted call row is one occurrence of. Every branch of one trigger is an
// alternative of it, which is why it is the trigger's column and not a
// branch's.
func (row CallCoordinate) ActivationApplication() (identity.SemanticKey, bool) {
	applicationID, ok := row.ApplicationID()
	if !ok {
		return identity.SemanticKey{}, false
	}
	return identity.NewSemanticKey([32]byte(applicationID), activationApplicationFrame)
}

// ActivationBranchForOccurrence is the branch directory's occurrence inverse: a
// branch IS a body, and a body is named by the module it resides in and the
// path it occupies there. It is the same pair every candidate row of this
// directory publishes, so the inverse and the projections agree by
// construction.
//
// It is asked once per declaration, never on a solve path.
func (algebra *Algebra) ActivationBranchForOccurrence(moduleID, bodyID identity.ContentID) (Body, bool) {
	if algebra == nil || !algebra.Valid() || !moduleID.Available() || !bodyID.Available() {
		return Body{}, false
	}
	bodies := algebra.Bodies()
	for index := 0; index < bodies.Count(); index++ {
		body, bodyOK := bodies.At(index)
		if !bodyOK {
			return Body{}, false
		}
		module, moduleOK := body.ModuleKey()
		path, pathOK := body.BodyPath()
		if !moduleOK || !pathOK {
			return Body{}, false
		}
		if module == moduleID && path == bodyID {
			return body, true
		}
	}
	return Body{}, false
}

// ActivationBranchOrdinal is the branch set's own dense directory: a body's
// position in Call's canonical target order.
func (algebra *Algebra) ActivationBranchOrdinal(body Body) (uint32, bool) {
	index, ok := algebra.Bodies().Index(body)
	if !ok || index < 0 {
		return 0, false
	}
	return uint32(index), true
}

// ActivationBranchAt addresses that directory.
func (algebra *Algebra) ActivationBranchAt(ordinal int) (Body, bool) {
	return algebra.Bodies().At(ordinal)
}

// ActivationTarget and ActivationEndpoint are the two semantic axes the
// transition one branch runs on connects. They are derived from the body's
// exact portable role, so a route's identity is the role's and never a
// position in a table.
func (body Body) ActivationTarget() (identity.SemanticKey, bool) {
	return body.activationRole(activationTargetFrame)
}

func (body Body) ActivationEndpoint() (identity.SemanticKey, bool) {
	return body.activationRole(activationEndpointFrame)
}

func (body Body) activationRole(frame uint64) (identity.SemanticKey, bool) {
	role, roleOK := body.RoleID()
	if !roleOK {
		return identity.SemanticKey{}, false
	}
	id, idOK := role.ContentID()
	if !idOK {
		return identity.SemanticKey{}, false
	}
	return identity.NewSemanticKey([32]byte(id), frame)
}
