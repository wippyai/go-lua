package call

import "github.com/wippyai/go-lua/analysis/program/target/vocabulary"

// TargetOperationKind is Call's closed classification of a known Target
// capability.  Invalid means the capability or its canonical role replay did
// not authenticate.  None means the capability is a valid Call alternative
// but does not select a Target operation (for example a function body or a
// non-operation seed).  Present carries the exact owner-issued operation.
type TargetOperationKind uint8

const (
	TargetOperationInvalid TargetOperationKind = iota
	TargetOperationNone
	TargetOperationPresent
)

// Valid reports whether the classification is one of the two authenticated
// target outcomes. Invalid is deliberately not a valid semantic result.
func (kind TargetOperationKind) Valid() bool {
	return kind == TargetOperationNone || kind == TargetOperationPresent
}

// ClassifyTargetOperation authenticates target through Call's canonical role
// directory before projecting its operation. The role round-trip is
// important: a Target with a matching-looking selector or operation from a
// foreign Algebra must remain Invalid, and no detached operation scalar may
// authorize a consumer.
//
// The returned operation is non-zero only for TargetOperationPresent. For
// TargetOperationNone and TargetOperationInvalid it is zero.
func (algebra *Algebra) ClassifyTargetOperation(target Target) (vocabulary.Operation, TargetOperationKind) {
	if algebra == nil || !algebra.Valid() || !algebra.OwnsTarget(target) {
		return 0, TargetOperationInvalid
	}
	role, roleOK := target.RoleID()
	canonicalRole, canonicalRoleOK := algebra.TargetForRole(role)
	canonicalTarget, canonicalTargetOK := canonicalRole.Target()
	canonicalRoleID, canonicalRoleIDOK := canonicalRole.ID()
	if !roleOK || !canonicalRoleOK || !canonicalTargetOK || !canonicalRoleIDOK ||
		!canonicalTarget.Same(target) || canonicalRoleID != role {
		return 0, TargetOperationInvalid
	}
	operation, operationOK := canonicalTarget.Operation()
	if operationOK {
		return operation, TargetOperationPresent
	}
	if canonicalRoleID.Kind() == TargetRoleBody || canonicalRoleID.Kind() == TargetRoleSeed {
		return 0, TargetOperationNone
	}
	return 0, TargetOperationInvalid
}
