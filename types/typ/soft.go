package typ

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// SoftPolicy controls how soft-placeholder detection behaves.
type SoftPolicy struct {
	// AllowEmptyRecord treats {} as a soft placeholder when true.
	AllowEmptyRecord bool
}

// SoftAnnotationPolicy treats empty records as non-soft (annotation semantics).
var SoftAnnotationPolicy = SoftPolicy{}

// SoftPlaceholderPolicy treats empty records as soft (placeholder semantics).
var SoftPlaceholderPolicy = SoftPolicy{AllowEmptyRecord: true}

// IsSoft reports whether a type should be treated as a soft placeholder under policy.
func IsSoft(t Type, policy SoftPolicy) bool {
	return isSoft(t, NewGuard(), policy)
}

func isSoft(t Type, guard internal.RecursionGuard, policy SoftPolicy) bool {
	return VisitWithGuard(t, guard, false, func(next internal.RecursionGuard) Visitor[bool] {
		return Visitor[bool]{
			Alias: func(a *Alias) bool {
				return isSoft(a.Target, next, policy)
			},
			Optional: func(o *Optional) bool {
				return isSoft(o.Inner, next, policy)
			},
			Array: func(a *Array) bool {
				return isSoft(a.Element, next, policy)
			},
			Map: func(m *Map) bool {
				return isSoft(m.Value, next, policy)
			},
			Record: func(r *Record) bool {
				if len(r.Fields) == 0 && !r.HasMapComponent() {
					return policy.AllowEmptyRecord
				}
				if r.HasMapComponent() && len(r.Fields) == 0 {
					return isSoft(r.MapValue, next, policy)
				}
				return false
			},
			Union: func(u *Union) bool {
				if len(u.Members) == 0 {
					return false
				}
				for _, m := range u.Members {
					if !isSoft(m, next, policy) {
						return false
					}
				}
				return true
			},
			Default: func(tt Type) bool {
				return tt.Kind().IsPlaceholder()
			},
		}
	})
}

// PruneSoftUnionMembers removes soft placeholder members from unions when
// any non-soft member is present. This helps inference avoid contaminating
// concrete types with placeholder unions.
func PruneSoftUnionMembers(t Type) Type {
	if t == nil {
		return nil
	}
	if !softPruneCanDescend(t) {
		return t
	}
	memo := make(map[Type]Type)
	visiting := make(map[Type]struct{})
	guard := NewGuard()
	return pruneSoftUnionMembersMemo(t, guard, memo, visiting)
}

func softPruneCanDescend(t Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Optional,
		kind.Union,
		kind.Array,
		kind.Map,
		kind.Tuple,
		kind.Function,
		kind.Record,
		kind.Alias,
		kind.Instantiated:
		return true
	default:
		return false
	}
}

func pruneSoftUnionMembersMemo(t Type, guard internal.RecursionGuard, memo map[Type]Type, visiting map[Type]struct{}) Type {
	if t == nil {
		return t
	}
	next, ok := guard.Enter(t)
	if !ok {
		return t
	}
	if cached, ok := memo[t]; ok {
		return cached
	}
	if _, ok := visiting[t]; ok {
		return t
	}
	visiting[t] = struct{}{}

	out := Visit(t, Visitor[Type]{
		Union: func(u *Union) Type {
			members := make([]Type, 0, len(u.Members))
			softFlags := make([]bool, 0, len(u.Members))
			softCount := 0
			nonSoftCount := 0
			changed := false
			for _, m := range u.Members {
				pm := pruneSoftUnionMembersMemo(m, next, memo, visiting)
				if pm != m {
					changed = true
				}
				soft := IsSoft(pm, SoftPlaceholderPolicy)
				softFlags = append(softFlags, soft)
				if soft {
					softCount++
				} else {
					nonSoftCount++
				}
				members = append(members, pm)
			}
			if softCount > 0 && nonSoftCount > 0 {
				filtered := members[:0]
				for i, m := range members {
					if !softFlags[i] {
						filtered = append(filtered, m)
					}
				}
				members = filtered
				changed = true
			}
			if !changed {
				return t
			}
			return NewUnion(members...)
		},
		Optional: func(o *Optional) Type {
			if o.Inner == nil {
				return t
			}
			inner := pruneSoftUnionMembersMemo(o.Inner, next, memo, visiting)
			if inner == o.Inner {
				return t
			}
			return NewOptional(inner)
		},
		Array: func(a *Array) Type {
			elem := pruneSoftUnionMembersMemo(a.Element, next, memo, visiting)
			if elem == a.Element {
				return t
			}
			return NewArray(elem)
		},
		Map: func(m *Map) Type {
			key := pruneSoftUnionMembersMemo(m.Key, next, memo, visiting)
			val := pruneSoftUnionMembersMemo(m.Value, next, memo, visiting)
			if key == m.Key && val == m.Value {
				return t
			}
			return NewMap(key, val)
		},
		Tuple: func(tu *Tuple) Type {
			changed := false
			elems := make([]Type, len(tu.Elements))
			for i, e := range tu.Elements {
				elems[i] = pruneSoftUnionMembersMemo(e, next, memo, visiting)
				if elems[i] != e {
					changed = true
				}
			}
			if !changed {
				return t
			}
			return NewTuple(elems...)
		},
		Function: func(f *Function) Type {
			return rewriteFunction(f, t, func(tt Type) (Type, bool) {
				return pruneSoftUnionMembersMemo(tt, next, memo, visiting), true
			}, next, nil)
		},
		Record: func(r *Record) Type {
			return rewriteRecord(r, t, func(tt Type) (Type, bool) {
				return pruneSoftUnionMembersMemo(tt, next, memo, visiting), true
			}, next, nil)
		},
		Alias: func(a *Alias) Type {
			target := pruneSoftUnionMembersMemo(a.Target, next, memo, visiting)
			if target == a.Target {
				return t
			}
			return NewAlias(a.Name, target)
		},
		Instantiated: func(i *Instantiated) Type {
			changed := false
			args := make([]Type, len(i.TypeArgs))
			for idx, a := range i.TypeArgs {
				args[idx] = pruneSoftUnionMembersMemo(a, next, memo, visiting)
				if args[idx] != a {
					changed = true
				}
			}
			if !changed {
				return t
			}
			return Instantiate(i.Generic, args...)
		},
		Default: func(_ Type) Type {
			return t
		},
	})

	delete(visiting, t)
	memo[t] = out
	return out
}
