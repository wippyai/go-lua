package typecontract

import "fmt"

// CallableAdmission is the closed callable shape requested by an operation
// declaration. The schema carries this request but never interprets a Type.
type CallableAdmission uint8

const (
	CallableAdmissionInvalid CallableAdmission = iota
	CallableAdmissionDirectFunction
	CallableAdmissionOrdinary
)

func (admission CallableAdmission) Available() bool {
	return admission == CallableAdmissionDirectFunction || admission == CallableAdmissionOrdinary
}

// FreshClass is the neutral runtime class requested by a fresh-result row.
// It is intentionally a portable row vocabulary; a domain adapter supplies
// the actual runtime/fresh judgment.
type FreshClass uint8

const (
	FreshClassInvalid FreshClass = iota
	FreshClassTable
	FreshClassFunction
	FreshClassThread
	FreshClassUserdata
	FreshClassError
	FreshClassReflection
)

func (class FreshClass) Available() bool {
	return class >= FreshClassTable && class <= FreshClassReflection
}

// Semantics is the typed admission algebra for one domain's neutral Type
// declarations. The schema owns this contract and the Type envelope; the
// domain owns the implementation. Formals are themselves neutral Type
// declarations, in operation-local ordinal order. An unavailable formal is
// unconstrained.
//
// Validate performs both wire validation and domain decode admission. The
// relation methods return an error for unavailable or undecodable operands;
// false is reserved for a valid but semantically rejected relation. This
// distinction prevents a structural-presence check from becoming a silent
// semantic fallback.
type Semantics interface {
	Validate(value Type, formals []Type) error
	Assignable(source, destination Type, formals []Type) (bool, error)
	Callable(value Type, admission CallableAdmission, formals []Type) (bool, error)
	Fresh(value Type, class FreshClass, formals []Type) (bool, error)
}

// ValidateSemantics rejects a missing implementation at composition. There is
// deliberately no default implementation or service lookup.
func ValidateSemantics(semantics Semantics) error {
	if semantics == nil {
		return fmt.Errorf("typecontract: semantic adapter is required")
	}
	return nil
}
