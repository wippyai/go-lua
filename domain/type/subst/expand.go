package subst

import (
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
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
	if t == nil || !typ.ContainsInstantiated(t) {
		return t
	}
	memo := getExpandMemo()
	defer putExpandMemo(memo)
	return expandInstantiatedGuardMode(t, &expandState{memo: memo}, expandModeStructural)
}

// ExpandInstantiatedRoot expands one generic application while preserving
// nested, unrelated applications as symbolic boundaries. Regular recurrence
// of the root application is closed into a finite recursive graph.
//
// This is the compositional form used by algorithms such as type inference:
// they need to inspect the root body without erasing the type arguments of
// nested generic applications.
func ExpandInstantiatedRoot(t typ.Type) typ.Type {
	if t == nil {
		return t
	}
	unwrapped := unwrap.Annotated(t)
	if _, ok := unwrapped.(*typ.Instantiated); !ok {
		return t
	}
	memo := getExpandMemo()
	defer putExpandMemo(memo)
	return expandInstantiatedGuardMode(t, &expandState{memo: memo}, expandModeRoot)
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
