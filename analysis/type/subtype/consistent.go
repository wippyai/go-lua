package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Consistent reports whether a source type may be assigned to a target type.
// Empty table literal source admission is explicit; use
// ConsistentFreshEmptyTable when the source expression is syntactically {}.
func Consistent(sub, super typ.Type) bool {
	if sub == nil || super == nil {
		return false
	}
	return IsSubtype(sub, super)
}

// ConsistentFreshEmptyTable reports whether a syntactic empty table literal ({})
// may be assigned to super. This source-only rule is intentionally not encoded
// in typ.Record or typ.Array structural identity.
func ConsistentFreshEmptyTable(super typ.Type) bool {
	return super != nil && emptyTableSatisfies(super)
}

// ConsistentSubtype is strict subtyping with explicit any acting as a source
// wildcard, used by generic-bound reconciliation.
func ConsistentSubtype(sub, super typ.Type) bool {
	if sub == nil || super == nil {
		return false
	}
	c := &checker{gradual: true}
	return c.check(sub, super, 0)
}

func emptyTableSatisfies(super typ.Type) bool {
	u := unwrap.Optional(super)
	if u == nil {
		return false
	}
	switch t := u.(type) {
	case *typ.Array:
		return true
	case *typ.Map:
		return true
	case *typ.ReadonlyMap:
		return true
	case *typ.Record:
		return recordAcceptsEmptyTable(t)
	case *typ.Tuple:
		return len(t.Elements) == 0
	case *typ.Union:
		for _, m := range t.Members {
			if emptyTableSatisfies(m) {
				return true
			}
		}
	case *typ.Intersection:
		if len(t.Members) == 0 {
			return false
		}
		for _, m := range t.Members {
			if !emptyTableSatisfies(m) {
				return false
			}
		}
		return true
	}
	return false
}

func recordAcceptsEmptyTable(r *typ.Record) bool {
	for _, f := range r.Fields {
		if !f.Optional {
			return false
		}
	}
	for _, m := range r.StaticMembers {
		if !m.Optional {
			return false
		}
	}
	return true
}
