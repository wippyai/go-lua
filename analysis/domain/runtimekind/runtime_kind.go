// Package runtimekind owns the closed Lua runtime type() vocabulary shared by
// analyzer domains.  It contains only immutable scalar vocabulary and set
// algebra: it is not a Factor and has no dependency on Value, Static, typ, or
// Program.
package runtimekind

// Kind is one observable Lua runtime family.
type Kind uint8

const (
	Invalid Kind = iota
	Nil
	Boolean
	Number
	String
	Table
	Function
	Thread
	Userdata
	Count
)

// Valid reports membership in the closed vocabulary.
func (kind Kind) Valid() bool { return kind > Invalid && kind < Count }

// Set is a may-set over the closed runtime vocabulary. Bottom is zero and All
// contains every known Lua runtime family.
type Set uint16

// Bit returns the singleton set for kind, or Bottom for an invalid kind.
func Bit(kind Kind) Set {
	if !kind.Valid() {
		return 0
	}
	return Set(1) << uint(kind-1)
}

const All Set = (Set(1) << uint(Count-1)) - 1

// Contains reports whether set includes the exact valid runtime family.
func (set Set) Contains(kind Kind) bool { return set&Bit(kind) != 0 }

// Valid reports that no out-of-vocabulary bit is set.
func (set Set) Valid() bool { return set&^All == 0 }
