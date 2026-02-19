// Package subst provides type substitution operations for generics.
//
// These operations replace type parameters with concrete types,
// used during generic instantiation and Self type resolution.
package subst

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// Substitute replaces type parameters with concrete types throughout a type.
//
// Used during generic instantiation to replace TypeParam references with
// the corresponding type arguments. The subs map keys are type parameter names.
func Substitute(t typ.Type, subs map[string]typ.Type) typ.Type {
	if len(subs) == 0 {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if tp, ok := n.(*typ.TypeParam); ok {
			if sub, ok := subs[tp.Name]; ok {
				return sub, true
			}
		}
		return nil, false
	})
}

// Params replaces type parameters with corresponding type arguments.
func Params(t typ.Type, params []*typ.TypeParam, args []typ.Type) typ.Type {
	if len(params) != len(args) || len(params) == 0 {
		return t
	}
	subs := make(map[string]typ.Type, len(params))
	for i, p := range params {
		subs[p.Name] = args[i]
	}
	return Substitute(t, subs)
}

// Self replaces Self type references with a concrete type.
// Does not recurse into Interface types because Self inside an Interface
// is a separate binding that refers to that Interface's implementor.
func Self(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		if _, ok := n.(*typ.Interface); ok {
			return n, true
		}
		return nil, false
	})
}

// ExpandInstantiated expands generic instantiations to their structural form.
//
// For Instantiated{Generic: Array<T>, TypeArgs: [number]}, returns the Array
// body with T replaced by number. This enables structural comparison and
// field/method lookup on instantiated generics.
//
// Does not enforce generic constraints; use subtype checking for that.
func ExpandInstantiated(t typ.Type) typ.Type {
	return expandInstantiatedWithDepth(t, typ.DeepRecursionDepth)
}

func expandInstantiatedWithDepth(t typ.Type, maxDepth int) typ.Type {
	guard := typ.GuardForDepth(maxDepth)
	return expandInstantiatedGuard(t, guard)
}

func expandInstantiatedGuard(t typ.Type, guard internal.RecursionGuard) typ.Type {
	return typ.VisitWithGuard(t, guard, t, func(next internal.RecursionGuard) typ.Visitor[typ.Type] {
		return typ.Visitor[typ.Type]{
			Instantiated: func(inst *typ.Instantiated) typ.Type {
				if inst.Generic == nil || len(inst.TypeArgs) != len(inst.Generic.TypeParams) {
					return t
				}
				if inst.Generic.Body == nil {
					return t
				}
				body := Params(inst.Generic.Body, inst.Generic.TypeParams, inst.TypeArgs)
				body = Self(body, t)
				return expandInstantiatedGuard(body, next)
			},
			Optional: func(o *typ.Optional) typ.Type {
				inner := expandInstantiatedGuard(o.Inner, next)
				if inner == o.Inner {
					return t
				}
				return typ.NewOptional(inner)
			},
			Union: func(u *typ.Union) typ.Type {
				var members []typ.Type
				for i, m := range u.Members {
					newMember := expandInstantiatedGuard(m, next)
					if newMember != m {
						if members == nil {
							members = make([]typ.Type, len(u.Members))
							copy(members, u.Members)
						}
						members[i] = newMember
					} else if members != nil {
						members[i] = m
					}
				}
				if members == nil {
					return t
				}
				return typ.NewUnion(members...)
			},
			Intersection: func(in *typ.Intersection) typ.Type {
				var members []typ.Type
				for i, m := range in.Members {
					newMember := expandInstantiatedGuard(m, next)
					if newMember != m {
						if members == nil {
							members = make([]typ.Type, len(in.Members))
							copy(members, in.Members)
						}
						members[i] = newMember
					} else if members != nil {
						members[i] = m
					}
				}
				if members == nil {
					return t
				}
				return typ.NewIntersection(members...)
			},
			Array: func(a *typ.Array) typ.Type {
				elem := expandInstantiatedGuard(a.Element, next)
				if elem == a.Element {
					return t
				}
				return typ.NewArray(elem)
			},
			Map: func(m *typ.Map) typ.Type {
				key := expandInstantiatedGuard(m.Key, next)
				value := expandInstantiatedGuard(m.Value, next)
				if key == m.Key && value == m.Value {
					return t
				}
				return typ.NewMap(key, value)
			},
			Tuple: func(tup *typ.Tuple) typ.Type {
				var elems []typ.Type
				for i, e := range tup.Elements {
					newElem := expandInstantiatedGuard(e, next)
					if newElem != e {
						if elems == nil {
							elems = make([]typ.Type, len(tup.Elements))
							copy(elems, tup.Elements)
						}
						elems[i] = newElem
					} else if elems != nil {
						elems[i] = e
					}
				}
				if elems == nil {
					return t
				}
				return typ.NewTuple(elems...)
			},
			Function: func(fn *typ.Function) typ.Type {
				changed := false
				var params []typ.Param
				for i, p := range fn.Params {
					newType := p.Type
					if _, isInst := p.Type.(*typ.Instantiated); !isInst {
						newType = expandInstantiatedGuard(p.Type, next)
					}
					if newType != p.Type {
						if params == nil {
							params = make([]typ.Param, len(fn.Params))
							copy(params, fn.Params)
						}
						changed = true
						params[i] = typ.Param{Name: p.Name, Type: newType, Optional: p.Optional}
					} else if params != nil {
						params[i] = p
					}
				}

				var returns []typ.Type
				for i, r := range fn.Returns {
					newRet := expandInstantiatedGuard(r, next)
					if newRet != r {
						if returns == nil {
							returns = make([]typ.Type, len(fn.Returns))
							copy(returns, fn.Returns)
						}
						changed = true
						returns[i] = newRet
					} else if returns != nil {
						returns[i] = r
					}
				}

				variadic := fn.Variadic
				if fn.Variadic != nil {
					newVariadic := expandInstantiatedGuard(fn.Variadic, next)
					if newVariadic != fn.Variadic {
						changed = true
						variadic = newVariadic
					}
				}

				if !changed {
					return t
				}

				builder := typ.Func()
				paramsSrc := fn.Params
				if params != nil {
					paramsSrc = params
				}
				for _, p := range paramsSrc {
					if p.Optional {
						builder = builder.OptParam(p.Name, p.Type)
					} else {
						builder = builder.Param(p.Name, p.Type)
					}
				}
				if variadic != nil {
					builder = builder.Variadic(variadic)
				}
				returnsSrc := fn.Returns
				if returns != nil {
					returnsSrc = returns
				}
				if len(returnsSrc) > 0 {
					builder = builder.Returns(returnsSrc...)
				}
				if fn.Effects != nil {
					builder = builder.Effects(fn.Effects)
				}
				if fn.Spec != nil {
					builder = builder.Spec(fn.Spec)
				}
				if fn.Refinement != nil {
					builder = builder.WithRefinement(fn.Refinement)
				}
				return builder.Build()
			},
			Record: func(rec *typ.Record) typ.Type {
				changed := false
				var fields []typ.Field
				for i, f := range rec.Fields {
					newType := expandInstantiatedGuard(f.Type, next)
					if newType != f.Type {
						if fields == nil {
							fields = make([]typ.Field, len(rec.Fields))
							copy(fields, rec.Fields)
						}
						changed = true
						fields[i] = typ.Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
					} else if fields != nil {
						fields[i] = f
					}
				}

				metatable := rec.Metatable
				if rec.Metatable != nil {
					newMetatable := expandInstantiatedGuard(rec.Metatable, next)
					if newMetatable != rec.Metatable {
						changed = true
						metatable = newMetatable
					}
				}

				mapKey := rec.MapKey
				mapValue := rec.MapValue
				if rec.HasMapComponent() {
					mapKey = expandInstantiatedGuard(rec.MapKey, next)
					if mapKey != rec.MapKey {
						changed = true
					}
					mapValue = expandInstantiatedGuard(rec.MapValue, next)
					if mapValue != rec.MapValue {
						changed = true
					}
				}

				if !changed {
					return t
				}

				builder := typ.NewRecord()
				if rec.Open {
					builder.SetOpen(true)
				}
				fieldsSrc := rec.Fields
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
			Alias: func(a *typ.Alias) typ.Type {
				target := expandInstantiatedGuard(a.Target, next)
				if target == a.Target {
					return t
				}
				return typ.NewAlias(a.Name, target)
			},
			Interface: func(i *typ.Interface) typ.Type {
				changed := false
				var methods []typ.Method
				for idx := range i.Methods {
					m := i.Methods[idx]
					newType := expandInstantiatedGuard(m.Type, next)
					fn, ok := newType.(*typ.Function)
					if !ok {
						fn = m.Type
					}
					if fn != m.Type {
						if methods == nil {
							methods = make([]typ.Method, len(i.Methods))
							copy(methods, i.Methods)
						}
						changed = true
						methods[idx] = typ.Method{Name: m.Name, Type: fn}
					} else if methods != nil {
						methods[idx] = m
					}
				}
				if !changed {
					return t
				}
				return typ.NewInterface(i.Name, methods)
			},
			Ref: func(r *typ.Ref) typ.Type {
				return t
			},
			Generic: func(g *typ.Generic) typ.Type {
				return t
			},
			Default: func(t typ.Type) typ.Type {
				return t
			},
		}
	})
}
