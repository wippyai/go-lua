package value

import (
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// HasHigherOrderGrowthRisk reports whether a type can produce non-monotone
// higher-order structural growth across abstract-interpretation iterations.
func HasHigherOrderGrowthRisk(t typ.Type) bool {
	if t == nil {
		return false
	}
	return Scan(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		switch n := node.(type) {
		case *typ.Function:
			for _, ret := range n.Returns {
				if containsFunction(ret) {
					return true, false
				}
			}
		case *typ.Record:
			if recordHasSelfRecursiveMethod(n) {
				return true, false
			}
		}
		return false, true
	})
}

func containsFunction(t typ.Type) bool {
	if t == nil {
		return false
	}
	return Scan(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if _, ok := node.(*typ.Interface); ok {
			return false, false
		}
		if _, ok := node.(*typ.Function); ok {
			return true, false
		}
		return false, true
	})
}

func recordHasSelfRecursiveMethod(r *typ.Record) bool {
	if r == nil {
		return false
	}
	for _, f := range r.Fields {
		if methodTypeHasSelfRecursiveReturn(f.Type, r) {
			return true
		}
	}
	return r.HasMapComponent() && methodTypeHasSelfRecursiveReturn(r.MapValue, r)
}

func methodTypeHasSelfRecursiveReturn(t typ.Type, owner *typ.Record) bool {
	if t == nil || owner == nil {
		return false
	}
	return Scan(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if _, ok := node.(*typ.Interface); ok {
			return false, false
		}
		fn, ok := node.(*typ.Function)
		if !ok {
			return false, true
		}
		for _, ret := range fn.Returns {
			if ret == nil {
				continue
			}
			if subtype.IsSubtype(ret, owner) || subtype.IsSubtype(owner, ret) ||
				ExtendsRecord(ret, owner) || ExtendsRecord(owner, ret) {
				return true, false
			}
		}
		return false, true
	})
}
