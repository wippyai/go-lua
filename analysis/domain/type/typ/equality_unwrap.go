package typ

import (
	"reflect"
)

func unwrapAliasForEquals(t Type) (Type, bool) {
	var seen typePath
	for {
		t = NormalizeNil(t)
		if t == nil {
			return nil, true
		}
		t = UnwrapTransparentWrappers(t)
		alias, ok := t.(*Alias)
		if !ok {
			return t, true
		}
		if !seen.enter(alias) {
			return nil, false
		}
		t = alias.UnaliasedTarget()
	}
}

func NormalizeNil(t Type) Type {
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
