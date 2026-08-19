package artifact

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
