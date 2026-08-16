package subst

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Self replaces Self type references with a concrete type.
// Does not recurse into Interface types because Self inside an Interface
// is a separate binding that refers to that Interface's implementor.
func Self(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	if !containsSubstitutableSelf(t) {
		return t
	}
	return rewriteSelf(t, selfType, selfRewriteMethodType)
}

type selfRewriteMode uint8

const (
	selfRewriteMethodType selfRewriteMode = iota
	selfRewriteRuntimeValue
)

func containsSubstitutableSelf(t typ.Type) bool {
	if t == nil {
		return false
	}
	memo := make(map[typ.Type]bool)
	stack := []typ.Type{t}
	for len(stack) != 0 {
		i := len(stack) - 1
		current := typ.UnwrapTransparentWrappers(stack[i])
		stack = stack[:i]
		if current == nil {
			continue
		}
		if current.Kind() == kind.Self {
			return true
		}
		// Recursive and Interface nodes each own the Self references in their
		// bodies.  Do not cross either binding boundary while looking for a
		// free Self in the enclosing type.
		if _, ok := current.(*typ.Interface); ok {
			continue
		}
		if _, ok := current.(*typ.Recursive); ok {
			continue
		}
		if _, seen := memo[current]; seen {
			continue
		}
		memo[current] = true
		// An instantiation owns only its arguments in the current scope; the
		// Generic declaration is a separate binder and is not a child here.
		if instantiated, ok := current.(*typ.Instantiated); ok {
			stack = append(stack, instantiated.TypeArgs...)
			continue
		}
		typ.WalkChildren(current, func(child typ.Type) bool {
			stack = append(stack, child)
			return false
		})
	}
	return false
}

// SelfValue replaces free Self references in a runtime value type. Nested
// function and interface types bind their own Self, so substitution stops at
// those boundaries.
func SelfValue(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return rewriteSelf(t, selfType, selfRewriteRuntimeValue)
}

func rewriteSelf(t typ.Type, selfType typ.Type, mode selfRewriteMode) typ.Type {
	return transform.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		if _, ok := n.(*typ.Interface); ok {
			return n, true
		}
		if mode == selfRewriteRuntimeValue {
			if _, ok := n.(*typ.Function); ok {
				return n, true
			}
		}
		return nil, false
	})
}
