package key

import "github.com/wippyai/go-lua/analysis/domain/formal"

// ValueDependency is one exact root of the Values factor referenced by a
// registered residual carrier. It is a closed sum: either a concrete State
// value cell or a vocabulary-qualified formal relation root. The zero value is
// invalid, so catalog visitors cannot silently manufacture an untyped root.
type ValueDependency struct {
	kind     valueDependencyKind
	concrete Value
	formal   formal.Root
}

type valueDependencyKind uint8

const (
	valueDependencyInvalid valueDependencyKind = iota
	valueDependencyConcrete
	valueDependencyFormal
)

// ConcreteDependency names one concrete Values cell.
func ConcreteDependency(value Value) ValueDependency {
	if value == 0 {
		return ValueDependency{}
	}
	return ValueDependency{kind: valueDependencyConcrete, concrete: value}
}

// FormalDependency names one exact root in a frozen relation vocabulary.
func FormalDependency(root formal.Root) ValueDependency {
	if !root.Valid() {
		return ValueDependency{}
	}
	return ValueDependency{kind: valueDependencyFormal, formal: root}
}

// Valid reports whether d is exactly one declared alternative.
func (d ValueDependency) Valid() bool {
	switch d.kind {
	case valueDependencyConcrete:
		return d.concrete != 0 && !d.formal.Valid()
	case valueDependencyFormal:
		return d.concrete == 0 && d.formal.Valid()
	default:
		return false
	}
}

// Concrete observes the concrete Values cell alternative.
func (d ValueDependency) Concrete() (Value, bool) {
	if d.kind != valueDependencyConcrete || !d.Valid() {
		return 0, false
	}
	return d.concrete, true
}

// Formal observes the formal relation root alternative.
func (d ValueDependency) Formal() (formal.Root, bool) {
	if d.kind != valueDependencyFormal || !d.Valid() {
		return formal.Root{}, false
	}
	return d.formal, true
}
