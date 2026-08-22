package call

import "github.com/wippyai/go-lua/analysis/program/target/vocabulary"

// TargetOperationKind is Call's closed classification of a known Target
// capability. Invalid means the owner-bound capability did not authenticate.
// None means the capability is a valid Call alternative
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

// ClassifyTargetOperation authenticates target through its exact Call owner
// before projecting its operation. Target already carries the unforgeable
// owner and selector capability; replaying its role through a second wrapper
// would add no authority.
//
// The returned operation is non-zero only for TargetOperationPresent. For
// TargetOperationNone and TargetOperationInvalid it is zero.
func (algebra *Algebra) ClassifyTargetOperation(target Target) (vocabulary.Operation, TargetOperationKind) {
	if algebra == nil || !algebra.Valid() || !algebra.OwnsTarget(target) {
		return 0, TargetOperationInvalid
	}
	role, roleOK := target.RoleID()
	if !roleOK {
		return 0, TargetOperationInvalid
	}
	operation, operationOK := target.Operation()
	if operationOK {
		return operation, TargetOperationPresent
	}
	if role.Kind() == TargetRoleBody || role.Kind() == TargetRoleSeed {
		return 0, TargetOperationNone
	}
	return 0, TargetOperationInvalid
}
