package typ

import "github.com/wippyai/go-lua/internal"

// Contains reports whether t or any nested type satisfies pred.
//
// This is the canonical structural scanner for type predicates. Callers should
// keep policy decisions at their own layer and use this only for traversal.
func Contains(t Type, pred func(Type) bool) bool {
	if pred == nil {
		return false
	}
	return contains(t, pred, make(containsSeen))
}

// ContainsAny reports whether t contains an explicit dynamic any type.
func ContainsAny(t Type) bool {
	return containsAnyType(t)
}

// ContainsNever reports whether t contains the bottom type as a nested product
// member. It uses construction-time containment flags for closed products.
func ContainsNever(t Type) bool {
	return containsNeverType(t)
}

// ContainsTypeParam reports whether t contains a type parameter.
func ContainsTypeParam(t Type) bool {
	return containsTypeParamType(t)
}

// ContainsInstantiated reports whether t contains a generic instantiation.
func ContainsInstantiated(t Type) bool {
	return containsInstantiatedType(t)
}

// ContainsRecursive reports whether t contains a recursive product. The answer
// uses construction-time flags for closed products and guarded recursive-body
// inspection for open placeholders.
func ContainsRecursive(t Type) bool {
	return knownContainsRecursive(t)
}

type containsSeen map[uint64][]Type

func (s containsSeen) contains(t Type) bool {
	if t == nil || s == nil {
		return false
	}
	for _, existing := range s[containsSeenKey(t)] {
		if SameNodeOrAcyclicEqual(existing, t) || SameProductFamily(existing, t) {
			return true
		}
	}
	return false
}

func (s containsSeen) remember(t Type) {
	if t == nil || s == nil {
		return
	}
	hash := containsSeenKey(t)
	s[hash] = append(s[hash], t)
}

func containsSeenKey(t Type) uint64 {
	if rec, ok := t.(*Recursive); ok {
		return internal.HashCombine(uint64(rec.Kind()), internal.FnvString(rec.Name))
	}
	return typeEqualityHash(t)
}

func contains(t Type, pred func(Type) bool, seen containsSeen) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if seen.contains(t) {
		return false
	}
	seen.remember(t)
	if pred(t) {
		return true
	}

	return Visit(t, Visitor[bool]{
		Optional: func(o *Optional) bool {
			return contains(o.Inner, pred, seen)
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if contains(member, pred, seen) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if contains(member, pred, seen) {
					return true
				}
			}
			return false
		},
		Array: func(a *Array) bool {
			return contains(a.Element, pred, seen)
		},
		Map: func(m *Map) bool {
			return contains(m.Key, pred, seen) ||
				contains(m.Value, pred, seen)
		},
		Tuple: func(tup *Tuple) bool {
			for _, elem := range tup.Elements {
				if contains(elem, pred, seen) {
					return true
				}
			}
			return false
		},
		Function: func(fn *Function) bool {
			for _, param := range fn.Params {
				if contains(param.Type, pred, seen) {
					return true
				}
			}
			if contains(fn.Variadic, pred, seen) {
				return true
			}
			for _, ret := range fn.Returns {
				if contains(ret, pred, seen) {
					return true
				}
			}
			for _, param := range fn.TypeParams {
				if param != nil && contains(param.Constraint, pred, seen) {
					return true
				}
			}
			return false
		},
		Record: func(r *Record) bool {
			if contains(r.MapKey, pred, seen) ||
				contains(r.MapValue, pred, seen) ||
				contains(r.Metatable, pred, seen) {
				return true
			}
			for _, field := range r.Fields {
				if contains(field.Type, pred, seen) {
					return true
				}
			}
			return false
		},
		Alias: func(a *Alias) bool {
			return contains(a.Target, pred, seen)
		},
		Meta: func(m *Meta) bool {
			return contains(m.Of, pred, seen)
		},
		Generic: func(g *Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && contains(param.Constraint, pred, seen) {
					return true
				}
			}
			return contains(g.Body, pred, seen)
		},
		Instantiated: func(i *Instantiated) bool {
			if i.Generic != nil && contains(i.Generic, pred, seen) {
				return true
			}
			for _, arg := range i.TypeArgs {
				if contains(arg, pred, seen) {
					return true
				}
			}
			return false
		},
		TypeParam: func(p *TypeParam) bool {
			return contains(p.Constraint, pred, seen)
		},
		FieldAccess: func(f *FieldAccess) bool {
			return contains(f.Base, pred, seen)
		},
		IndexAccess: func(i *IndexAccess) bool {
			return contains(i.Base, pred, seen) ||
				contains(i.Index, pred, seen)
		},
		Sum: func(s *Sum) bool {
			for _, variant := range s.Variants {
				for _, t := range variant.Types {
					if contains(t, pred, seen) {
						return true
					}
				}
			}
			return false
		},
		Interface: func(i *Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && contains(method.Type, pred, seen) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *Recursive) bool {
			return contains(r.Body, pred, seen)
		},
		Default: func(Type) bool {
			return false
		},
	})
}
