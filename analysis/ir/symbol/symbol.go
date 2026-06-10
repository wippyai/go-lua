// Package symbol defines lexical declaration identity for analysis IR.
package symbol

import "sync/atomic"

// ID uniquely identifies a lexical declaration across the program.
//
// ID 0 is reserved for unresolved or unknown references.
type ID uint64

var counter uint64

// Next generates a unique symbol ID.
func Next() ID {
	return ID(atomic.AddUint64(&counter, 1))
}

// Reserve reserves a contiguous block of symbol IDs and returns the first ID.
// It returns 0 when n <= 0.
func Reserve(n int) ID {
	if n <= 0 {
		return 0
	}
	end := atomic.AddUint64(&counter, uint64(n))
	start := end - uint64(n) + 1
	return ID(start)
}

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
)
