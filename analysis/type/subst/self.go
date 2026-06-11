package subst

import (
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Self replaces Self type references with a concrete type.
// Does not recurse into Interface types because Self inside an Interface
// is a separate binding that refers to that Interface's implementor.
func Self(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	if !containsSubstitutableSelf(t, make(map[typ.Type]bool)) {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		if _, ok := n.(*typ.Interface); ok {
			return n, true
		}
		return nil, false
	})
}

func containsSubstitutableSelf(t typ.Type, seen map[typ.Type]bool) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return false
	}
	if t.Kind() == kind.Self {
		return true
	}
	if _, ok := t.(*typ.Interface); ok {
		return false
	}
	if _, ok := t.(*typ.Recursive); ok {
		return false
	}
	if inspect.ContainsRecursive(t) && !containsSurfaceSelf(t) {
		return false
	}
	if seen[t] {
		return false
	}
	if len(seen) >= selfSubstitutionNodeBudget {
		return false
	}
	seen[t] = true
	switch v := t.(type) {
	case *typ.Optional:
		return containsSubstitutableSelf(v.Inner, seen)
	case *typ.Union:
		for _, member := range v.Members {
			if containsSubstitutableSelf(member, seen) {
				return true
			}
		}
	case *typ.Intersection:
		for _, member := range v.Members {
			if containsSubstitutableSelf(member, seen) {
				return true
			}
		}
	case *typ.Array:
		return containsSubstitutableSelf(v.Element, seen)
	case *typ.Map:
		return containsSubstitutableSelf(v.Key, seen) ||
			containsSubstitutableSelf(v.Value, seen)
	case *typ.ReadonlyMap:
		return containsSubstitutableSelf(v.Key, seen) ||
			containsSubstitutableSelf(v.Value, seen)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if containsSubstitutableSelf(elem, seen) {
				return true
			}
		}
	case *typ.Function:
		for _, param := range v.Params {
			if containsSubstitutableSelf(param.Type, seen) {
				return true
			}
		}
		if containsSubstitutableSelf(v.Variadic, seen) {
			return true
		}
		for _, ret := range v.Returns {
			if containsSubstitutableSelf(ret, seen) {
				return true
			}
		}
	case *typ.Record:
		for _, field := range v.Fields {
			if containsSubstitutableSelf(field.Type, seen) {
				return true
			}
		}
		for _, member := range v.StaticMembers {
			if containsSubstitutableSelf(member.Type, seen) {
				return true
			}
		}
		if containsSubstitutableSelf(v.Metatable, seen) {
			return true
		}
		if v.HasMapComponent() {
			return containsSubstitutableSelf(v.MapKey, seen) ||
				containsSubstitutableSelf(v.MapValue, seen)
		}
	case *typ.Alias:
		return containsSubstitutableSelf(v.Target, seen)
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if containsSubstitutableSelf(arg, seen) {
				return true
			}
		}
	}
	return false
}

func containsSurfaceSelf(t typ.Type) bool {
	return containsSurfaceSelfSeen(t, make(map[typ.Type]bool))
}

func containsSurfaceSelfSeen(t typ.Type, seen map[typ.Type]bool) bool {
	t = unwrap.Annotated(t)
	if t == nil {
		return false
	}
	if t.Kind() == kind.Self {
		return true
	}
	if _, ok := t.(*typ.Interface); ok {
		return false
	}
	if _, ok := t.(*typ.Recursive); ok {
		return false
	}
	if seen[t] {
		return false
	}
	if len(seen) >= selfSubstitutionNodeBudget {
		return false
	}
	seen[t] = true
	switch v := t.(type) {
	case *typ.Optional:
		return containsSurfaceSelfSeen(v.Inner, seen)
	case *typ.Union:
		for _, member := range v.Members {
			if containsSurfaceSelfSeen(member, seen) {
				return true
			}
		}
	case *typ.Intersection:
		for _, member := range v.Members {
			if containsSurfaceSelfSeen(member, seen) {
				return true
			}
		}
	case *typ.Array:
		return containsSurfaceSelfSeen(v.Element, seen)
	case *typ.Map:
		return containsSurfaceSelfSeen(v.Key, seen) || containsSurfaceSelfSeen(v.Value, seen)
	case *typ.ReadonlyMap:
		return containsSurfaceSelfSeen(v.Key, seen) || containsSurfaceSelfSeen(v.Value, seen)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if containsSurfaceSelfSeen(elem, seen) {
				return true
			}
		}
	case *typ.Function:
		for _, param := range v.Params {
			if containsSurfaceSelfSeen(param.Type, seen) {
				return true
			}
		}
		if containsSurfaceSelfSeen(v.Variadic, seen) {
			return true
		}
		for _, ret := range v.Returns {
			if containsSurfaceSelfSeen(ret, seen) {
				return true
			}
		}
	case *typ.Record:
		for _, field := range v.Fields {
			if containsSurfaceSelfSeen(field.Type, seen) {
				return true
			}
		}
		for _, member := range v.StaticMembers {
			if containsSurfaceSelfSeen(member.Type, seen) {
				return true
			}
		}
		if containsSurfaceSelfSeen(v.Metatable, seen) {
			return true
		}
		if v.HasMapComponent() {
			return containsSurfaceSelfSeen(v.MapKey, seen) || containsSurfaceSelfSeen(v.MapValue, seen)
		}
	case *typ.Alias:
		return containsSurfaceSelfSeen(v.Target, seen)
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if containsSurfaceSelfSeen(arg, seen) {
				return true
			}
		}
	}
	return false
}

const selfSubstitutionNodeBudget = 2048

// SelfValue replaces free Self references in a runtime value type. Nested
// function and interface types bind their own Self, so substitution stops at
// those boundaries.
func SelfValue(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		switch n.(type) {
		case *typ.Function, *typ.Interface:
			return n, true
		}
		return nil, false
	})
}
