// Package symbol defines lexical declaration identity for analysis.
package symbol

// ID identifies a lexical declaration within one binding pass.
//
// IDs are allocated bind-locally so identical source yields identical IDs
// across independent solves. They are unique within a single binding Result and
// are never compared against symbols produced by another Result.
//
// ID 0 is reserved for unresolved or unknown references.
type ID uint64

// Kind classifies how a symbol was declared.
type Kind int

const (
	// Unknown indicates the symbol kind is not known.
	Unknown Kind = iota
	// Param indicates a function parameter.
	Param
	// Local indicates a local variable.
	Local
	// Global indicates a global variable.
	Global
	// Upvalue indicates an upvalue captured from an enclosing scope.
	Upvalue
	// Function indicates a function expression identity.
	Function
)
