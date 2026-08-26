package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Kind is the closed logical expression vocabulary. Positive recursion is
// represented by plan dependency edges, never by another expression kind.
type Kind uint8

const (
	KindInvalid Kind = iota
	KindInput
	KindSelect
	KindProject
	KindJoin
	KindMerge
	KindGroup
	KindComplete
	KindApply
	KindPublish
	// KindColumnProject is a closed positional projection of an already
	// typed row.  Unlike Project, it does not construct a target relation or
	// look up nominal cells: its slots redeem exact child cell occurrences.
	KindColumnProject
	// KindExpand is a dependent keyed join over an owner-sealed finite vector.
	// Its contract carries logical identities only; mount evidence freezes the
	// vector contents separately.
	KindExpand
)

// KindCount is the number of admitted expression kinds. It excludes the zero
// invalid value.
const KindCount = int(KindExpand)

// Kinds returns the complete vocabulary in canonical order.
func Kinds() []Kind {
	return []Kind{KindInput, KindSelect, KindProject, KindJoin, KindMerge, KindGroup, KindComplete, KindApply, KindPublish, KindColumnProject, KindExpand}
}

// Expression is the sealed immutable expression interface. The interface is
// intentionally small: Digest is the one canonical structural identity
// surface, while the checker owns all semantic validity rules.
type Expression interface {
	Kind() Kind
	Digest() identity.ContentID
	expression()
}
