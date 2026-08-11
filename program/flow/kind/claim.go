package kind

// ValueClaimKind preserves the exact authored spelling of a scalar value
// claim. Validation, narrowing, guards, and outcome behavior are later
// derived relations, not authored Flow content.
type ValueClaimKind uint8

const (
	ValueClaimTypeAs ValueClaimKind = iota + 1
	ValueClaimTypeColonColon
	ValueClaimNonNil
)
