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
)

// KindCount is the number of admitted expression kinds. It excludes the zero
// invalid value.
const KindCount = int(KindPublish)

// Kinds returns the complete vocabulary in canonical order.
func Kinds() []Kind {
	return []Kind{KindInput, KindSelect, KindProject, KindJoin, KindMerge, KindGroup, KindComplete, KindApply, KindPublish}
}

// Expression is the sealed immutable expression interface. The interface is
// intentionally small: Digest is the one canonical structural identity
// surface, while the checker owns all semantic validity rules.
type Expression interface {
	Kind() Kind
	Digest() identity.ContentID
	expression()
}
