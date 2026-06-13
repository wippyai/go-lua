package typ

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/internal/recursion"
)

func unwrapAliasForEquals(t Type, guard recursion.Guard) Type {
	for {
		t = normalizeNilType(t)
		if t == nil {
			return nil
		}
		t = unwrapTransparentWrappers(t)
		next, ok := guard.Enter(t)
		if !ok {
			return nil
		}
		guard = next

		alias, ok := t.(*Alias)
		if !ok {
			return t
		}
		t = alias.UnaliasedTarget()
	}
}

// NormalizeNilType converts typed nil Type implementations to nil.
func NormalizeNilType(t Type) Type {
	return normalizeNilType(t)
}

func normalizeNilType(t Type) Type {
	if t == nil {
		return nil
	}
	v := reflect.ValueOf(t)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil
		}
	}
	return t
}
