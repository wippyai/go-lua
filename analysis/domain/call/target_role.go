package call

import "github.com/wippyai/go-lua/program/keyspace"

// TargetRoleKind distinguishes the two semantic target families owned by
// Call. The kind is part of the role identity so a body and seed with equal
// underlying content bytes cannot alias.
type TargetRoleKind uint8

const (
	TargetRoleInvalid TargetRoleKind = iota
	TargetRoleBody
	TargetRoleSeed
)

// TargetRoleID is the portable identity of one Call target role. It contains
// no Algebra, Link, Project, Boundary, or dense target selector authority.
// The private constructor is the only route for issuing one from a live
// owner; replay uses Algebra.TargetForRole.
type TargetRoleID struct {
	kind TargetRoleKind
	id   keyspace.ContentID
}

func newTargetRoleID(kind TargetRoleKind, id keyspace.ContentID) (TargetRoleID, bool) {
	if (kind != TargetRoleBody && kind != TargetRoleSeed) || !id.Available() {
		return TargetRoleID{}, false
	}
	return TargetRoleID{kind: kind, id: id}, true
}

func (role TargetRoleID) Valid() bool {
	return (role.kind == TargetRoleBody || role.kind == TargetRoleSeed) && role.id.Available()
}

func (role TargetRoleID) Kind() TargetRoleKind { return role.kind }

// ContentID returns the stable semantic target identity without exposing any
// live owner or dense coordinate.
func (role TargetRoleID) ContentID() (keyspace.ContentID, bool) {
	if !role.Valid() {
		return keyspace.ContentID{}, false
	}
	return role.id, true
}

// TargetRole is an owner-issued hot proof for one TargetRoleID. Equal role
// IDs from equivalent Algebras intentionally remain distinct proofs because
// owner identity is retained here, outside the portable ID.
type TargetRole struct {
	owner    *Algebra
	id       TargetRoleID
	selector selector
}

func (role TargetRole) Valid() bool {
	if role.owner == nil || !role.id.Valid() || !role.selector.valid() || uint64(role.selector) > uint64(len(role.owner.targets)) || role.owner.roleIndex[role.id] != role.selector {
		return false
	}
	return role.owner.targets[role.selector-1].role == role.id
}

func (role TargetRole) ID() (TargetRoleID, bool) {
	if !role.Valid() {
		return TargetRoleID{}, false
	}
	return role.id, true
}

// Target projects the exact owner-fenced target capability represented by
// this role proof. It does not expose the selector ordinal.
func (role TargetRole) Target() (Target, bool) {
	if !role.Valid() {
		return Target{}, false
	}
	return Target{owner: role.owner, selector: role.selector}, true
}

// OwnsRole authenticates both the hot Algebra owner and the canonical role
// index. A role from an equal-but-foreign Algebra therefore fails closed.
func (algebra *Algebra) OwnsRole(role TargetRole) bool {
	return algebra != nil && role.owner == algebra && role.Valid()
}

// RoleID projects a target capability into its stable semantic role identity.
func (target Target) RoleID() (TargetRoleID, bool) {
	if !target.Valid() {
		return TargetRoleID{}, false
	}
	return target.owner.targets[target.selector-1].role, true
}

// Role issues the owner-authenticated role proof for one target capability.
func (target Target) Role() (TargetRole, bool) {
	if !target.Valid() {
		return TargetRole{}, false
	}
	id, ok := target.RoleID()
	if !ok || !id.Valid() {
		return TargetRole{}, false
	}
	return TargetRole{owner: target.owner, id: id, selector: target.selector}, true
}

// RoleID projects a Body capability using its exact target row. It is useful
// to callers that intentionally retain only the body role identity.
func (body Body) RoleID() (TargetRoleID, bool) {
	if !body.Valid() || body.owner == nil {
		return TargetRoleID{}, false
	}
	selector := body.owner.targetIndex[targetKey{kind: targetBody, moduleKey: body.module, bodyContext: body.context}]
	if !selector.valid() || uint64(selector) > uint64(len(body.owner.targets)) {
		return TargetRoleID{}, false
	}
	target := Target{owner: body.owner, selector: selector}
	return target.RoleID()
}

// TargetForRole replays a portable role ID into this exact Algebra owner.
// Equivalent Algebras may accept the same ID, but the returned proof and
// Target remain owned by this Algebra and cannot cross the original owner.
func (algebra *Algebra) TargetForRole(id TargetRoleID) (TargetRole, bool) {
	if algebra == nil || !algebra.Valid() || !id.Valid() {
		return TargetRole{}, false
	}
	selector := algebra.roleIndex[id]
	if !selector.valid() || uint64(selector) > uint64(len(algebra.targets)) || algebra.targets[selector-1].role != id {
		return TargetRole{}, false
	}
	return TargetRole{owner: algebra, id: id, selector: selector}, true
}
