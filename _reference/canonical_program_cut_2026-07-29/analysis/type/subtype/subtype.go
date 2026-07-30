package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// IsSubtype reports whether sub is a strict subtype of super.
func IsSubtype(sub, super typ.Type) bool {
	c := &checker{}
	return c.check(sub, super, 0)
}

// IsFreshAssignable reports whether sub is a subtype of super or widens to it
// under fresh-constructor rules (record->map, singleton literal->base/union,
// array->map, and structurally through those). A fresh table literal assigned to
// an annotated location adopts that location's declared type when this holds, so
// nested literal fields take their declared contract rather than their narrow
// constructed type.
func IsFreshAssignable(sub, super typ.Type) bool {
	if (&checker{}).check(sub, super, 0) {
		return true
	}
	// Widening is a distinct recursive relation. Its coinductive assumptions
	// must not observe false memo entries from the preceding subtype proof.
	return (&checker{}).canWidenTo(sub, super, 0)
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
