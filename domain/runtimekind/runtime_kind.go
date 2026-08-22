// Package runtimekind owns the closed Lua runtime type() vocabulary shared by
// analyzer domains.  It contains only immutable scalar vocabulary and set
// algebra: it is not a Factor and has no dependency on Value, Static, typ, or
// Program.
package runtimekind

import "math/bits"

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

// kindSpellings is the sole spelling authority for the closed vocabulary.
// Its indices are Kind ordinals; structural registration and any diagnostic
// projection read this owner-held description instead of restating the
// Kind-to-name relation.
var kindSpellings = [Count]string{
	Nil:      "nil",
	Boolean:  "boolean",
	Number:   "number",
	String:   "string",
	Table:    "table",
	Function: "function",
	Thread:   "thread",
	Userdata: "userdata",
}

// Valid reports membership in the closed vocabulary.
func (kind Kind) Valid() bool { return kind > Invalid && kind < Count }

// Spelling returns the exact string produced by Lua type() for kind. Invalid
// kinds have no spelling.
func (kind Kind) Spelling() string {
	if !kind.Valid() {
		return ""
	}
	return kindSpellings[kind]
}

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

// The vocabulary's partitions. Every subset of families the analyzer reasons
// about as a group is named once, here, so a consumer selects a partition by
// name instead of restating its member list or walking an ordinal range that
// mistranslates the moment a family is inserted into Kind.
//
// Each term is the singleton Bit of the family it names, written as a constant
// expression because Go admits no constant call. The partition laws state the
// equality against Bit itself, so the two spellings cannot diverge.
const (
	// Reference is every family whose values are references: equality on them
	// is object identity and one of them may retain a graph.
	Reference Set = Set(1)<<(Table-1) | Set(1)<<(Function-1) | Set(1)<<(Thread-1) | Set(1)<<(Userdata-1)

	// Scalar is every non-nil family whose values carry no referenced object.
	Scalar Set = Set(1)<<(Boolean-1) | Set(1)<<(Number-1) | Set(1)<<(String-1)

	// Opaque is the reference families whose contents the analyzer models no
	// structure for.
	Opaque Set = Set(1)<<(Thread-1) | Set(1)<<(Userdata-1)

	// NonNil is the closed vocabulary less Lua nil: the families a value has
	// once its presence is proved.
	NonNil Set = All &^ (Set(1) << (Nil - 1))
)

// Contains reports whether set includes the exact valid runtime family.
func (set Set) Contains(kind Kind) bool { return set&Bit(kind) != 0 }

// Valid reports that no out-of-vocabulary bit is set.
func (set Set) Valid() bool { return set&^All == 0 }

// Members reports how many families set contains.
func (set Set) Members() int {
	if !set.Valid() {
		return 0
	}
	return bits.OnesCount16(uint16(set))
}

// MemberAt returns the family at position index of set in vocabulary order,
// and false once the walk is exhausted. It is the one enumeration of a set, so
// a consumer visiting every family projects it rather than restating an
// ordinal range. The walk costs no allocation.
func (set Set) MemberAt(index int) (Kind, bool) {
	if index < 0 || !set.Valid() {
		return Invalid, false
	}
	for kind := Invalid + 1; kind < Count; kind++ {
		if !set.Contains(kind) {
			continue
		}
		if index == 0 {
			return kind, true
		}
		index--
	}
	return Invalid, false
}
