package subst

import (
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ExpandInstantiated expands generic instantiations to their Lua
// table-normalized structural form.
//
// For Instantiated{Generic: Array<T>, TypeArgs: [number]}, returns the Array
// body with T replaced by number. This enables structural comparison and
// field/method lookup on instantiated generics.
//
// Does not enforce generic constraints; use subtype checking for that.
func ExpandInstantiated(t typ.Type) typ.Type {
	if t == nil || !inspect.ContainsInstantiated(t) {
		return t
	}
	memo := getExpandMemo()
	defer putExpandMemo(memo)
	guard := typ.GuardForDepth(typ.DefaultRecursionDepth)
	return expandInstantiatedGuardMode(t, guard, memo, expandModeStructural)
}

// ExpandInstantiatedChanged expands generic instantiations and reports whether
// the result differs from the input.
func ExpandInstantiatedChanged(t typ.Type) (typ.Type, bool) {
	expanded := ExpandInstantiated(t)
	if expanded == nil || expanded == t {
		return nil, false
	}
	return expanded, true
}
