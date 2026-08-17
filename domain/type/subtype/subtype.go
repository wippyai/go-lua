package subtype

import (
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// IsSubtype reports whether sub is a strict subtype of super.
func IsSubtype(sub, super typ.Type) bool {
	c := &checker{}
	return c.check(sub, super)
}

// IsFreshAssignable reports whether sub is a subtype of super or widens to it
// under fresh-constructor rules (record->map, singleton literal->base/union,
// array->map, and structurally through those). A fresh table literal assigned to
// an annotated location adopts that location's declared type when this holds, so
// nested literal fields take their declared contract rather than their narrow
// constructed type.
func IsFreshAssignable(sub, super typ.Type) bool {
	if (&checker{}).check(sub, super) {
		return true
	}
	// Widening is a distinct recursive relation. Its coinductive assumptions
	// must not observe false memo entries from the preceding subtype proof.
	return (&checker{}).canWidenTo(sub, super)
}

func isOptionalTop(t typ.Type) bool {
	opt, ok := unwrap.Alias(t).(*typ.Optional)
	if !ok || opt == nil {
		return false
	}
	inner := unwrap.Alias(opt.Inner)
	return typ.IsAny(inner) || typ.IsUnknown(inner)
}

type checker struct {
	inProgress      map[typePair]bool
	memo            map[typePair]bool
	widenInProgress map[typePair]bool
	widenMemo       map[typePair]bool
}

// Batch is a sealed prover for many independent top-level judgments. Every
// judgment starts from an empty memo, so a coinductive assumption or a result
// recorded under one can never be observed by another; only the already
// allocated table storage is reused. It answers exactly what IsSubtype
// answers and exists so a seal-time materialization of the relation does not
// pay a fresh prover allocation per pair.
//
// A Batch is not safe for concurrent use.
type Batch struct{ prover checker }

func (b *Batch) IsSubtype(sub, super typ.Type) bool {
	if b == nil {
		return IsSubtype(sub, super)
	}
	b.prover.reset()
	return b.prover.check(sub, super)
}

func (c *checker) reset() {
	clear(c.inProgress)
	clear(c.memo)
	clear(c.widenInProgress)
	clear(c.widenMemo)
}
