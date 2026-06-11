package identity

import (
	"reflect"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// SameNode reports whether two Type interface values point at the same
// immutable type node. It is intentionally not structural equality; callers
// use it to detect no-op rewrites without walking recursive products.
func SameNode(a, b typ.Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)
	if va.Type() != vb.Type() || !va.Type().Comparable() {
		return false
	}
	return a == b
}

// TypeEquals compares two types for structural equality with cycle detection.
//
// Uses coinductive equality for recursive types: if the same type pair is
// encountered again during traversal, they are assumed equal. This handles
// infinite recursive structures correctly.
//
// Aliases are transparent: compares through to their targets.
func TypeEquals(a, b typ.Type) bool {
	if a == b {
		return true
	}
	return typeEqualsGuard(a, b, typ.NewGuard(), nil)
}

// SameNodeOrAcyclicEqual reports identity or structural equality for products
// that cannot contain recursive cycles. Recursive product-family equivalence is
// a domain relation; generic constructors and hot convergence paths must not
// prove it by unfolding structural equality.
func SameNodeOrAcyclicEqual(a, b typ.Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if typ.ContainsRecursive(a) || typ.ContainsRecursive(b) {
		return false
	}
	return TypeEquals(a, b)
}

func typeEqualsGuard(a, b typ.Type, guard recursion.Guard, seen map[typePair]bool) bool {
	a = normalizeNilType(a)
	b = normalizeNilType(b)
	a = unwrapAliasForEquals(a, guard)
	b = unwrapAliasForEquals(b, guard)

	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if ra, ok := a.(*typ.Ref); ok {
		if rb, ok := b.(*typ.Ref); ok {
			return ra.Module == rb.Module && ra.Name == rb.Name
		}
		return false
	}
	if _, ok := b.(*typ.Ref); ok {
		return false
	}

	if a.Kind() != b.Kind() {
		return false
	}
	if typeEqualsCanUseHashPrefilter(a, b) && EqualityHash(a) != EqualityHash(b) {
		return false
	}

	if needsCycleCheck(a.Kind()) {
		ap := typePointer(a)
		bp := typePointer(b)
		if ap != 0 && bp != 0 {
			pair := typePair{a: ap, b: bp}
			if seen == nil {
				seen = make(map[typePair]bool)
			}
			if seen[pair] {
				return true
			}
			seen[pair] = true
		}
	}

	next := guard
	a = unwrap.Annotated(a)
	b = unwrap.Annotated(b)

	switch va := a.(type) {
	case *typ.Optional:
		vb, ok := b.(*typ.Optional)
		return ok && typeEqualsGuard(va.Inner, vb.Inner, next, seen)
	case *typ.Union:
		vb, ok := b.(*typ.Union)
		if !ok || len(va.Members) != len(vb.Members) {
			return false
		}
		for i, m := range va.Members {
			if !typeEqualsGuard(m, vb.Members[i], next, seen) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		vb, ok := b.(*typ.Intersection)
		if !ok || len(va.Members) != len(vb.Members) {
			return false
		}
		for i, m := range va.Members {
			if !typeEqualsGuard(m, vb.Members[i], next, seen) {
				return false
			}
		}
		return true
	case *typ.Tuple:
		vb, ok := b.(*typ.Tuple)
		if !ok || len(va.Elements) != len(vb.Elements) {
			return false
		}
		for i, e := range va.Elements {
			if !typeEqualsGuard(e, vb.Elements[i], next, seen) {
				return false
			}
		}
		return true
	case *typ.Array:
		vb, ok := b.(*typ.Array)
		return ok && typeEqualsGuard(va.Element, vb.Element, next, seen)
	case *typ.Map:
		vb, ok := b.(*typ.Map)
		return ok &&
			typeEqualsGuard(va.Key, vb.Key, next, seen) &&
			typeEqualsGuard(va.Value, vb.Value, next, seen)
	case *typ.ReadonlyMap:
		vb, ok := b.(*typ.ReadonlyMap)
		return ok &&
			typeEqualsGuard(va.Key, vb.Key, next, seen) &&
			typeEqualsGuard(va.Value, vb.Value, next, seen)
	case *typ.Record:
		vb, ok := b.(*typ.Record)
		if !ok || va.Open != vb.Open ||
			len(va.Fields) != len(vb.Fields) ||
			len(va.StaticMembers) != len(vb.StaticMembers) {
			return false
		}
		for i, f := range va.Fields {
			fb := vb.Fields[i]
			if f.Name != fb.Name || f.Optional != fb.Optional || f.Readonly != fb.Readonly {
				return false
			}
			if !typeEqualsGuard(f.Type, fb.Type, next, seen) {
				return false
			}
		}
		for i, m := range va.StaticMembers {
			mb := vb.StaticMembers[i]
			if m.Kind != mb.Kind || m.Name != mb.Name || m.Index != mb.Index ||
				m.Optional != mb.Optional || m.Readonly != mb.Readonly {
				return false
			}
			if !typeEqualsGuard(m.Type, mb.Type, next, seen) {
				return false
			}
		}
		if va.HasMapComponent() != vb.HasMapComponent() {
			return false
		}
		if va.HasMapComponent() {
			if !typeEqualsGuard(va.MapKey, vb.MapKey, next, seen) {
				return false
			}
			if !typeEqualsGuard(va.MapValue, vb.MapValue, next, seen) {
				return false
			}
		}
		if (va.Metatable == nil) != (vb.Metatable == nil) {
			return false
		}
		if va.Metatable != nil && !typeEqualsGuard(va.Metatable, vb.Metatable, next, seen) {
			return false
		}
		return true
	case *typ.Function:
		vb, ok := b.(*typ.Function)
		if !ok || len(va.TypeParams) != len(vb.TypeParams) {
			return false
		}
		for i, tp := range va.TypeParams {
			if !typeEqualsGuard(tp, vb.TypeParams[i], next, seen) {
				return false
			}
		}
		if len(va.Params) != len(vb.Params) || len(va.Returns) != len(vb.Returns) {
			return false
		}
		for i, p := range va.Params {
			pb := vb.Params[i]
			if p.Optional != pb.Optional {
				return false
			}
			if !typeEqualsGuard(p.Type, pb.Type, next, seen) {
				return false
			}
		}
		if (va.Variadic == nil) != (vb.Variadic == nil) {
			return false
		}
		if va.Variadic != nil && !typeEqualsGuard(va.Variadic, vb.Variadic, next, seen) {
			return false
		}
		for i, r := range va.Returns {
			if !typeEqualsGuard(r, vb.Returns[i], next, seen) {
				return false
			}
		}
		return true
	case *typ.Generic:
		vb, ok := b.(*typ.Generic)
		if !ok || va.Name != vb.Name || len(va.TypeParams) != len(vb.TypeParams) {
			return false
		}
		for i, tp := range va.TypeParams {
			if !tp.Equals(vb.TypeParams[i]) {
				return false
			}
		}
		if (va.Body == nil) != (vb.Body == nil) {
			return false
		}
		return va.Body == nil || typeEqualsGuard(va.Body, vb.Body, next, seen)
	case *typ.Instantiated:
		vb, ok := b.(*typ.Instantiated)
		if !ok || !typeEqualsGuard(va.Generic, vb.Generic, next, seen) || len(va.TypeArgs) != len(vb.TypeArgs) {
			return false
		}
		for i, arg := range va.TypeArgs {
			if !typeEqualsGuard(arg, vb.TypeArgs[i], next, seen) {
				return false
			}
		}
		return true
	case *typ.Recursive:
		vb, ok := b.(*typ.Recursive)
		if !ok {
			return false
		}
		if va.ID == vb.ID {
			return true
		}
		if va.Name != vb.Name {
			return false
		}
		return typeEqualsGuard(va.Body, vb.Body, next, seen)
	default:
		return a.Equals(b)
	}
}

// NormalizeNilType converts typed nil Type implementations to nil.
func NormalizeNilType(t typ.Type) typ.Type {
	return normalizeNilType(t)
}

func unwrapAliasForEquals(t typ.Type, guard recursion.Guard) typ.Type {
	for {
		t = normalizeNilType(t)
		if t == nil {
			return nil
		}
		t = unwrap.Annotated(t)
		next, ok := guard.Enter(t)
		if !ok {
			return nil
		}
		guard = next

		alias, ok := t.(*typ.Alias)
		if !ok {
			return t
		}
		t = alias.UnaliasedTarget()
	}
}

func normalizeNilType(t typ.Type) typ.Type {
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

func typeEqualsCanUseHashPrefilter(a, b typ.Type) bool {
	return !typ.ContainsRecursive(a) && !typ.ContainsRecursive(b)
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

func typePointer(t typ.Type) uintptr {
	switch tt := t.(type) {
	case *typ.Union:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Intersection:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Record:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Function:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Generic:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Instantiated:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Interface:
		return uintptr(unsafe.Pointer(tt))
	case *typ.Recursive:
		return uintptr(unsafe.Pointer(tt))
	}

	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0
	}
	return v.Pointer()
}
