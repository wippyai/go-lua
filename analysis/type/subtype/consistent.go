package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Consistent reports whether a source type may be assigned to a target type.
// It is strict subtyping plus the fresh empty-table source admission.
func Consistent(sub, super typ.Type) bool {
	if sub == nil || super == nil {
		return false
	}
	return IsSubtype(sub, super) || ConsistentBeyondSubtype(sub, super)
}

func ConsistentBeyondSubtype(sub, super typ.Type) bool {
	if sub == nil || super == nil {
		return false
	}
	return isFreshEmptyTable(sub) && emptyTableSatisfies(super)
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

func isFreshEmptyTable(t typ.Type) bool {
	switch tt := t.(type) {
	case *typ.Record:
		return tt.Fresh
	case *typ.Array:
		return tt.Fresh
	}
	return false
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
