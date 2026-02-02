package typ

import (
	"reflect"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// TypeEquals compares two types for structural equality with cycle detection.
//
// Uses coinductive equality for recursive types: if the same type pair is
// encountered again during traversal, they are assumed equal. This handles
// infinite recursive structures correctly.
//
// Aliases are transparent: compares through to their targets.
func TypeEquals(a, b Type) bool {
	guard := NewGuard()
	return typeEqualsGuard(a, b, guard, nil)
}

func typeEqualsGuard(a, b Type, guard internal.RecursionGuard, seen map[typePair]bool) bool {
	if a == b {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	// Unwrap Ref types
	if ra, ok := a.(*Ref); ok {
		if rb, ok := b.(*Ref); ok {
			return ra.Module == rb.Module && ra.Name == rb.Name
		}

		return false
	}

	if _, ok := b.(*Ref); ok {
		return false
	}

	// Unwrap Alias types
	if aa, ok := a.(*Alias); ok {
		next, ok := guard.Enter(a)
		if !ok {
			return false
		}
		return typeEqualsGuard(aa.Target, b, next, seen)
	}

	if ab, ok := b.(*Alias); ok {
		next, ok := guard.Enter(b)
		if !ok {
			return false
		}
		return typeEqualsGuard(a, ab.Target, next, seen)
	}

	if a.Kind() != b.Kind() {
		return false
	}

	// For compound types, use pointer-based cycle detection to avoid hash collisions.
	if needsCycleCheck(a.Kind()) {
		ap := typePointer(a)
		bp := typePointer(b)

		if ap != 0 && bp != 0 {
			pair := typePair{a: ap, b: bp}

			if seen == nil {
				seen = make(map[typePair]bool)
			}

			if seen[pair] {
				return true // cycle, coinductively equal
			}

			seen[pair] = true
		}
	}

	return VisitWithGuard(a, guard, false, func(next internal.RecursionGuard) Visitor[bool] {
		return Visitor[bool]{
			Optional: func(o *Optional) bool {
				return typeEqualsGuard(o.Inner, b.(*Optional).Inner, next, seen)
			},
			Union: func(u *Union) bool {
				vb := b.(*Union)
				if len(u.Members) != len(vb.Members) {
					return false
				}
				for i, m := range u.Members {
					if !typeEqualsGuard(m, vb.Members[i], next, seen) {
						return false
					}
				}
				return true
			},
			Intersection: func(u *Intersection) bool {
				vb := b.(*Intersection)
				if len(u.Members) != len(vb.Members) {
					return false
				}
				for i, m := range u.Members {
					if !typeEqualsGuard(m, vb.Members[i], next, seen) {
						return false
					}
				}
				return true
			},
			Tuple: func(tu *Tuple) bool {
				vb := b.(*Tuple)
				if len(tu.Elements) != len(vb.Elements) {
					return false
				}
				for i, e := range tu.Elements {
					if !typeEqualsGuard(e, vb.Elements[i], next, seen) {
						return false
					}
				}
				return true
			},
			Array: func(a *Array) bool {
				return typeEqualsGuard(a.Element, b.(*Array).Element, next, seen)
			},
			Map: func(m *Map) bool {
				vb := b.(*Map)
				return typeEqualsGuard(m.Key, vb.Key, next, seen) &&
					typeEqualsGuard(m.Value, vb.Value, next, seen)
			},
			Record: func(r *Record) bool {
				vb := b.(*Record)
				if r.Open != vb.Open {
					return false
				}
				if len(r.Fields) != len(vb.Fields) {
					return false
				}
				for i, f := range r.Fields {
					fb := vb.Fields[i]
					if f.Name != fb.Name || f.Optional != fb.Optional || f.Readonly != fb.Readonly {
						return false
					}
					if !typeEqualsGuard(f.Type, fb.Type, next, seen) {
						return false
					}
				}
				if r.HasMapComponent() != vb.HasMapComponent() {
					return false
				}
				if r.HasMapComponent() {
					if !typeEqualsGuard(r.MapKey, vb.MapKey, next, seen) {
						return false
					}
					if !typeEqualsGuard(r.MapValue, vb.MapValue, next, seen) {
						return false
					}
				}
				return true
			},
			Function: func(fn *Function) bool {
				vb := b.(*Function)
				if len(fn.Params) != len(vb.Params) || len(fn.Returns) != len(vb.Returns) {
					return false
				}
				for i, p := range fn.Params {
					pb := vb.Params[i]
					if p.Optional != pb.Optional {
						return false
					}
					if !typeEqualsGuard(p.Type, pb.Type, next, seen) {
						return false
					}
				}
				if (fn.Variadic == nil) != (vb.Variadic == nil) {
					return false
				}
				if fn.Variadic != nil && !typeEqualsGuard(fn.Variadic, vb.Variadic, next, seen) {
					return false
				}
				for i, r := range fn.Returns {
					if !typeEqualsGuard(r, vb.Returns[i], next, seen) {
						return false
					}
				}
				return true
			},
			Generic: func(g *Generic) bool {
				vb := b.(*Generic)
				if g.Name != vb.Name || len(g.TypeParams) != len(vb.TypeParams) {
					return false
				}
				for i, tp := range g.TypeParams {
					if !tp.Equals(vb.TypeParams[i]) {
						return false
					}
				}
				if g.Name != "" {
					return true
				}
				if (g.Body == nil) != (vb.Body == nil) {
					return false
				}
				if g.Body != nil && !typeEqualsGuard(g.Body, vb.Body, next, seen) {
					return false
				}
				return true
			},
			Instantiated: func(i *Instantiated) bool {
				vb := b.(*Instantiated)
				if !typeEqualsGuard(i.Generic, vb.Generic, next, seen) {
					return false
				}
				if len(i.TypeArgs) != len(vb.TypeArgs) {
					return false
				}
				for idx, arg := range i.TypeArgs {
					if !typeEqualsGuard(arg, vb.TypeArgs[idx], next, seen) {
						return false
					}
				}
				return true
			},
			Recursive: func(r *Recursive) bool {
				vb, ok := b.(*Recursive)
				if !ok {
					return false
				}
				if r.ID == vb.ID {
					return true
				}
				if r.Name != vb.Name {
					return false
				}
				return typeEqualsGuard(r.Body, vb.Body, next, seen)
			},
			Default: func(_ Type) bool {
				return a.Equals(b)
			},
		}
	})
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
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0
	}

	return v.Pointer()
}

// TypeString returns string representation with depth limiting.
func TypeString(t Type) string {
	guard := NewGuard()
	return typeStringGuard(t, guard)
}

func typeStringGuard(t Type, guard internal.RecursionGuard) string {
	if t == nil {
		return "nil"
	}
	return WithGuard(t, guard, "...", func(internal.RecursionGuard) string {
		return t.String()
	})
}
