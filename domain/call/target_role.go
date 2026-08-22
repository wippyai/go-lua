package call

import "github.com/wippyai/go-lua/analysis/identity"

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
	id   identity.ContentID
}

func newTargetRoleID(kind TargetRoleKind, id identity.ContentID) (TargetRoleID, bool) {
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
func (role TargetRoleID) ContentID() (identity.ContentID, bool) {
	if !role.Valid() {
		return identity.ContentID{}, false
	}
	return role.id, true
}

// RoleID projects a target capability into its stable semantic role identity.
func (target Target) RoleID() (TargetRoleID, bool) {
	if !target.Valid() {
		return TargetRoleID{}, false
	}
	return target.owner.targets[target.selector-1].role, true
}

// RoleID projects a Body capability using its exact target row. It is useful
// to callers that intentionally retain only the body role identity.
func (body Body) RoleID() (TargetRoleID, bool) {
	if !body.Valid() || body.owner == nil {
		return TargetRoleID{}, false
	}
	target := Target{owner: body.owner, selector: body.selector}
	return target.RoleID()
}

// TargetForRole replays a portable role ID directly into this exact Algebra's
// owner-fenced target capability. The role index is the sole inverse; no
// role-specific hot wrapper is introduced between the identity and Target.
func (algebra *Algebra) TargetForRole(id TargetRoleID) (Target, bool) {
	if algebra == nil || !algebra.Valid() || !id.Valid() {
		return Target{}, false
	}
	selector := algebra.roleIndex[id]
	if !selector.valid() || uint64(selector) > uint64(len(algebra.targets)) || algebra.targets[selector-1].role != id {
		return Target{}, false
	}
	return Target{owner: algebra, selector: selector}, true
}
