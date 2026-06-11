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

func isOptionalTop(t typ.Type) bool {
	opt, ok := unwrap.Alias(t).(*typ.Optional)
	if !ok || opt == nil {
		return false
	}
	inner := unwrap.Alias(opt.Inner)
	return typ.IsAny(inner) || typ.IsUnknown(inner)
}

type checker struct {
	inProgress map[typePair]bool
	memo       map[typePair]bool
}
