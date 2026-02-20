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
	if t == nil {
		return false
	}
	node := unwrapTransparentSoft(t)
	switch node.(type) {
	case *Alias, *Optional, *Array, *Map, *Record, *Union:
		// recurse below
	default:
		return node.Kind().IsPlaceholder()
	}

	next, ok := guard.Enter(node)
	if !ok {
		return false
	}

	switch tt := node.(type) {
	case *Alias:
		return isSoft(tt.Target, next, policy)
	case *Optional:
		return isSoft(tt.Inner, next, policy)
	case *Array:
		return isSoft(tt.Element, next, policy)
	case *Map:
		return isSoft(tt.Value, next, policy)
	case *Record:
		if len(tt.Fields) == 0 && !tt.HasMapComponent() {
			return policy.AllowEmptyRecord
		}
		if tt.HasMapComponent() && len(tt.Fields) == 0 {
			return isSoft(tt.MapValue, next, policy)
		}
		return false
	case *Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, m := range tt.Members {
			if !isSoft(m, next, policy) {
				return false
			}
		}
		return true
	}
	return false
}

func unwrapTransparentSoft(t Type) Type {
	for {
		ann, ok := t.(*Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
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
	softMemo := make(map[Type]bool)
	guard := NewGuard()
	return pruneSoftUnionMembersMemo(t, guard, memo, visiting, softMemo)
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

func pruneSoftUnionMembersMemo(
	t Type,
	guard internal.RecursionGuard,
	memo map[Type]Type,
	visiting map[Type]struct{},
	softMemo map[Type]bool,
) Type {
	if t == nil {
		return t
	}
	if !softPruneCanDescend(t) {
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
			var rewritten []Type
			nonSoftMembers := make([]Type, 0, len(u.Members))
			softCount := 0
			nonSoftCount := 0
			changed := false
			for idx, m := range u.Members {
				pm := pruneSoftUnionMembersMemo(m, next, memo, visiting, softMemo)
				if pm != m {
					if rewritten == nil {
						rewritten = make([]Type, len(u.Members))
						copy(rewritten, u.Members)
					}
					rewritten[idx] = pm
					changed = true
				} else if rewritten != nil {
					rewritten[idx] = m
				}
				soft := isSoftWithMemo(pm, SoftPlaceholderPolicy, softMemo)
				if soft {
					softCount++
				} else {
					nonSoftCount++
					nonSoftMembers = append(nonSoftMembers, pm)
				}
			}
			if softCount > 0 && nonSoftCount > 0 {
				return NewUnion(nonSoftMembers...)
			}
			if !changed {
				return t
			}
			members := u.Members
			if rewritten != nil {
				members = rewritten
			}
			return NewUnion(members...)
		},
		Optional: func(o *Optional) Type {
			if o.Inner == nil {
				return t
			}
			inner := pruneSoftUnionMembersMemo(o.Inner, next, memo, visiting, softMemo)
			if inner == o.Inner {
				return t
			}
			return NewOptional(inner)
		},
		Array: func(a *Array) Type {
			elem := pruneSoftUnionMembersMemo(a.Element, next, memo, visiting, softMemo)
			if elem == a.Element {
				return t
			}
			return NewArray(elem)
		},
		Map: func(m *Map) Type {
			key := pruneSoftUnionMembersMemo(m.Key, next, memo, visiting, softMemo)
			val := pruneSoftUnionMembersMemo(m.Value, next, memo, visiting, softMemo)
			if key == m.Key && val == m.Value {
				return t
			}
			return NewMap(key, val)
		},
		Tuple: func(tu *Tuple) Type {
			var elems []Type
			for i, e := range tu.Elements {
				newElem := pruneSoftUnionMembersMemo(e, next, memo, visiting, softMemo)
				if newElem != e {
					if elems == nil {
						elems = make([]Type, len(tu.Elements))
						copy(elems, tu.Elements)
					}
					elems[i] = newElem
				} else if elems != nil {
					elems[i] = e
				}
			}
			if elems == nil {
				return t
			}
			return NewTuple(elems...)
		},
		Function: func(f *Function) Type {
			changed := false

			var params []Param
			for i, p := range f.Params {
				newType := pruneSoftUnionMembersMemo(p.Type, next, memo, visiting, softMemo)
				if newType != p.Type {
					if params == nil {
						params = make([]Param, len(f.Params))
						copy(params, f.Params)
					}
					changed = true
					params[i] = Param{Name: p.Name, Type: newType, Optional: p.Optional}
				} else if params != nil {
					params[i] = p
				}
			}

			var returns []Type
			for i, r := range f.Returns {
				newRet := pruneSoftUnionMembersMemo(r, next, memo, visiting, softMemo)
				if newRet != r {
					if returns == nil {
						returns = make([]Type, len(f.Returns))
						copy(returns, f.Returns)
					}
					changed = true
					returns[i] = newRet
				} else if returns != nil {
					returns[i] = r
				}
			}

			variadic := f.Variadic
			if f.Variadic != nil {
				newVariadic := pruneSoftUnionMembersMemo(f.Variadic, next, memo, visiting, softMemo)
				if newVariadic != f.Variadic {
					changed = true
					variadic = newVariadic
				}
			}

			if !changed {
				return t
			}

			builder := Func()
			paramSrc := f.Params
			if params != nil {
				paramSrc = params
			}
			for _, p := range paramSrc {
				if p.Optional {
					builder = builder.OptParam(p.Name, p.Type)
				} else {
					builder = builder.Param(p.Name, p.Type)
				}
			}
			if variadic != nil {
				builder = builder.Variadic(variadic)
			}
			returnsSrc := f.Returns
			if returns != nil {
				returnsSrc = returns
			}
			if len(returnsSrc) > 0 {
				builder = builder.Returns(returnsSrc...)
			}
			if f.Effects != nil {
				builder = builder.Effects(f.Effects)
			}
			if f.Spec != nil {
				builder = builder.Spec(f.Spec)
			}
			if f.Refinement != nil {
				builder = builder.WithRefinement(f.Refinement)
			}
			return builder.Build()
		},
		Record: func(r *Record) Type {
			changed := false

			var fields []Field
			for i, f := range r.Fields {
				newType := pruneSoftUnionMembersMemo(f.Type, next, memo, visiting, softMemo)
				if newType != f.Type {
					if fields == nil {
						fields = make([]Field, len(r.Fields))
						copy(fields, r.Fields)
					}
					changed = true
					fields[i] = Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
				} else if fields != nil {
					fields[i] = f
				}
			}

			metatable := r.Metatable
			if r.Metatable != nil {
				newMetatable := pruneSoftUnionMembersMemo(r.Metatable, next, memo, visiting, softMemo)
				if newMetatable != r.Metatable {
					changed = true
					metatable = newMetatable
				}
			}

			mapKey := r.MapKey
			mapValue := r.MapValue
			if r.HasMapComponent() {
				newMapKey := pruneSoftUnionMembersMemo(r.MapKey, next, memo, visiting, softMemo)
				if newMapKey != r.MapKey {
					changed = true
					mapKey = newMapKey
				}
				newMapValue := pruneSoftUnionMembersMemo(r.MapValue, next, memo, visiting, softMemo)
				if newMapValue != r.MapValue {
					changed = true
					mapValue = newMapValue
				}
			}

			if !changed {
				return t
			}

			builder := NewRecord()
			if r.Open {
				builder.SetOpen(true)
			}
			fieldsSrc := r.Fields
			if fields != nil {
				fieldsSrc = fields
			}
			for _, f := range fieldsSrc {
				switch {
				case f.Optional && f.Readonly:
					builder = builder.OptReadonlyField(f.Name, f.Type)
				case f.Optional:
					builder = builder.OptField(f.Name, f.Type)
				case f.Readonly:
					builder = builder.ReadonlyField(f.Name, f.Type)
				default:
					builder = builder.Field(f.Name, f.Type)
				}
			}
			if metatable != nil {
				builder = builder.Metatable(metatable)
			}
			if mapKey != nil && mapValue != nil {
				builder = builder.MapComponent(mapKey, mapValue)
			}
			return builder.Build()
		},
		Alias: func(a *Alias) Type {
			target := pruneSoftUnionMembersMemo(a.Target, next, memo, visiting, softMemo)
			if target == a.Target {
				return t
			}
			return NewAlias(a.Name, target)
		},
		Instantiated: func(i *Instantiated) Type {
			var args []Type
			for idx, a := range i.TypeArgs {
				newArg := pruneSoftUnionMembersMemo(a, next, memo, visiting, softMemo)
				if newArg != a {
					if args == nil {
						args = make([]Type, len(i.TypeArgs))
						copy(args, i.TypeArgs)
					}
					args[idx] = newArg
				} else if args != nil {
					args[idx] = a
				}
			}
			if args == nil {
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

func isSoftWithMemo(t Type, policy SoftPolicy, memo map[Type]bool) bool {
	if t == nil {
		return false
	}
	if cached, ok := memo[t]; ok {
		return cached
	}
	node := unwrapTransparentSoft(t)
	if node != t {
		if cached, ok := memo[node]; ok {
			memo[t] = cached
			return cached
		}
	}
	soft := IsSoft(node, policy)
	memo[t] = soft
	if node != t {
		memo[node] = soft
	}
	return soft
}
