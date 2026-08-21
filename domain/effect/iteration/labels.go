package iteration

import (
	"fmt"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
)

var _ effect.Label = Iterator{}

// IteratorKind is one member of the closed vocabulary of iteration protocols
// this package owns. The ordinals are dense from IterateIndexed and are this
// package's own numbering. They are not a wire format: a serializer that needs
// a stable external spelling declares one against this catalog rather than
// writing the ordinal or reading off the display spelling.
type IteratorKind int

const (
	IterateIndexed IteratorKind = iota
	IterateKeyed
	iteratorKindLimit
)

// IteratorKindCount is the size of the closed vocabulary. The ordinals are
// dense from IterateIndexed, so a consumer indexes by kind without a lookup.
const IteratorKindCount = int(iteratorKindLimit)

// Valid reports membership in the closed vocabulary.
func (kind IteratorKind) Valid() bool {
	return kind >= IterateIndexed && kind < iteratorKindLimit
}

// IteratorKinds is the vocabulary catalog in ordinal order. It is the one
// enumeration of the kinds this package owns, so a consumer that visits,
// serializes, or declares every kind projects it instead of restating the
// member list. The catalog is returned by value and costs no allocation to
// range over.
func IteratorKinds() [IteratorKindCount]IteratorKind {
	return [IteratorKindCount]IteratorKind{IterateIndexed, IterateKeyed}
}

// String is the display spelling of a kind and the one place it is written, so
// the label that carries a kind renders through it rather than restating the
// member list.
func (kind IteratorKind) String() string {
	switch kind {
	case IterateIndexed:
		return "indexed"
	case IterateKeyed:
		return "keyed"
	default:
		return "unknown"
	}
}

type Iterator struct {
	Source effect.ParamRef
	Kind   IteratorKind
}

func (Iterator) CapabilityID() string { return capability.IterationIterator }
func (i Iterator) String() string {
	return fmt.Sprintf("iterator(%s, %s)", i.Source, i.Kind)
}
func (i Iterator) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(Iterator); ok {
		return i.Source.Index == o.Source.Index && i.Kind == o.Kind
	}
	return false
}
