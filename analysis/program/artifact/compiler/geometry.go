package compiler

// allocationRole is compiler-only assembly state for one authored allocation
// family. Publication converts it to the canonical Program vocabulary.
type allocationRole uint8

const (
	allocationInvalid allocationRole = iota
	allocationTable
	allocationClosure
)

func (role allocationRole) Valid() bool {
	return role == allocationTable || role == allocationClosure
}

// allocationForm is compiler-only constructor geometry. Its ordinals match
// the occurrence code written into the canonical Program publication.
type allocationForm uint8

const (
	allocationFormInvalid allocationForm = iota
	allocationFormEmpty
	allocationFormClosed
	allocationFormFinalOpen
)

func (form allocationForm) Valid() bool {
	return form >= allocationFormEmpty && form <= allocationFormFinalOpen
}
