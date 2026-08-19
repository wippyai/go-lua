package artifact

import "github.com/wippyai/go-lua/analysis/program/flow"

// AllocationRole is the receipt column for one authored allocation family.
// Compile copies it from Flow once; retained rows never re-export Flow types.
type AllocationRole uint8

const (
	AllocationInvalid AllocationRole = iota
	AllocationTable
	AllocationClosure
)

func (role AllocationRole) Valid() bool {
	return role == AllocationTable || role == AllocationClosure
}

// AllocationForm is the receipt column for one allocation's constructor
// geometry. Ordinals match the occurrence Code the compiler writes.
type AllocationForm uint8

const (
	AllocationFormInvalid AllocationForm = iota
	AllocationFormEmpty
	AllocationFormClosed
	AllocationFormFinalOpen
)

func (form AllocationForm) Valid() bool {
	return form >= AllocationFormEmpty && form <= AllocationFormFinalOpen
}

// CallForm is the receipt column for one authored call shape.
type CallForm uint8

const (
	CallFormInvalid CallForm = iota
	CallFormPlain
	CallFormMethod
)

func (form CallForm) Valid() bool {
	return form == CallFormPlain || form == CallFormMethod
}

func receiptCallForm(form flow.CallForm) (CallForm, bool) {
	switch form {
	case flow.CallFormPlain:
		return CallFormPlain, true
	case flow.CallFormMethod:
		return CallFormMethod, true
	default:
		return CallFormInvalid, false
	}
}
