package member

import "github.com/wippyai/go-lua/analysis/schema"

// Addressing names the columns of one relation that its own rows are
// addressed by.
//
// A join onto a relation pairs a source row against a column of that
// relation. The relation is the only authority on which of its columns that
// is, so it names them here and every addressing coordinate becomes an
// ordinary declared column: a projection this catalog declares over this
// relation, resolvable by name like any other. Parent, ordinal, tag and
// occurrence stop being roles a reader has to know how to reconstruct and
// become data a reader resolves.
//
// A relation declares the coordinates it actually has. Silence is a verdict:
// a relation that names no tag column publishes no tag, and a selection that
// needs one has nothing to pair against rather than a guessed counterpart.
type Addressing struct {
	// Address is the column one row of this relation is identified by. It is
	// the side an oriented equijoin onto this relation pairs a source row
	// against.
	Address schema.Key
	// Parent is the column carrying the address of the parent row this row
	// hangs off. It is declared exactly when the relation declares a Parent.
	Parent schema.Key
	// Ordinal is the column that keys this row within its parent's member set.
	// It is declared exactly when Parent is: a parent with no ordinal gives
	// its members no address, and an ordinal with no parent keys nothing.
	Ordinal schema.Key
	// Tag is the column a selection correlates one returned row with the
	// source row that selected it.
	Tag schema.Key
	// Occurrence is the column naming the occurrence family a row is
	// enumerated under. A directory holding rows drawn from several
	// occurrence families declares one; a directory whose rows are all one
	// family declares none, because its occurrence is the candidate's own.
	Occurrence schema.Key
}

// Declared reports whether this relation names any addressing column.
func (addressing Addressing) Declared() bool {
	return addressing.Address.Available() || addressing.Parent.Available() ||
		addressing.Ordinal.Available() || addressing.Tag.Available() ||
		addressing.Occurrence.Available()
}

// Columns returns every declared addressing column in a stable order, so a
// reader validates or resolves them without spelling the five names again.
func (addressing Addressing) Columns() []schema.Key {
	declared := make([]schema.Key, 0, 5)
	for _, column := range [...]schema.Key{
		addressing.Address, addressing.Parent, addressing.Ordinal,
		addressing.Tag, addressing.Occurrence,
	} {
		if column.Available() {
			declared = append(declared, column)
		}
	}
	return declared
}

// consistent reports whether the declared addressing agrees with the nesting
// the relation states elsewhere. The parent and ordinal columns stand or fall
// together with the parent relation, the same biconditional the ordinal
// carrier already answers to, and one column may not fill two roles.
func (addressing Addressing) consistent(nested bool) bool {
	if addressing.Parent.Available() != addressing.Ordinal.Available() {
		return false
	}
	if addressing.Parent.Available() && !nested {
		return false
	}
	seen := make(map[schema.Key]struct{}, 5)
	for _, column := range addressing.Columns() {
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}
