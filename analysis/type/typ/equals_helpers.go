package typ

import (
	"reflect"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/kind"
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

func typeEqualsCanUseHashPrefilter(a, b Type) bool {
	return !knownContainsRecursive(a) &&
		!knownContainsRecursive(b) &&
		!knownContainsOpenRecursive(a) &&
		!knownContainsOpenRecursive(b)
}

func needsCycleCheck(k kind.Kind) bool {
	switch k {
	case kind.Union, kind.Intersection, kind.Record, kind.Function,
		kind.Generic, kind.Instantiated, kind.Interface, kind.Recursive:
		return true
	}

	return false
}

type typePair struct {
	a uintptr
	b uintptr
}

func typePointer(t Type) uintptr {
	switch tt := t.(type) {
	case *Union:
		return uintptr(unsafe.Pointer(tt))
	case *Intersection:
		return uintptr(unsafe.Pointer(tt))
	case *Record:
		return uintptr(unsafe.Pointer(tt))
	case *Function:
		return uintptr(unsafe.Pointer(tt))
	case *Generic:
		return uintptr(unsafe.Pointer(tt))
	case *Instantiated:
		return uintptr(unsafe.Pointer(tt))
	case *Interface:
		return uintptr(unsafe.Pointer(tt))
	case *Recursive:
		return uintptr(unsafe.Pointer(tt))
	}

	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0
	}

	return v.Pointer()
}
