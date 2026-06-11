package inspect

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Contains reports whether t or any nested type satisfies pred.
func Contains(t typ.Type, pred func(typ.Type) bool) bool {
	if pred == nil {
		return false
	}
	return contains(t, pred, make(containsSeen))
}

// ContainsAny reports whether t contains an explicit dynamic any type.
func ContainsAny(t typ.Type) bool {
	return Contains(t, typ.IsAny)
}

// ContainsNever reports whether t contains the bottom type as a nested member.
func ContainsNever(t typ.Type) bool {
	return Contains(t, typ.IsNever)
}

// ContainsTypeParam reports whether t contains a type parameter.
func ContainsTypeParam(t typ.Type) bool {
	return Contains(t, func(t typ.Type) bool {
		_, ok := t.(*typ.TypeParam)
		return ok
	})
}

// ContainsInstantiated reports whether t contains a generic instantiation.
func ContainsInstantiated(t typ.Type) bool {
	return Contains(t, func(t typ.Type) bool {
		_, ok := t.(*typ.Instantiated)
		return ok
	})
}

// ContainsRecursive reports whether t contains a recursive product.
func ContainsRecursive(t typ.Type) bool {
	return containsRecursive(t, make(containsRecursiveSeen))
}

type containsRecursiveSeen map[typ.Type]struct{}

func (s containsRecursiveSeen) contains(t typ.Type) bool {
	if !containsRecursiveSeenTracks(t) {
		return false
	}
	_, ok := s[t]
	return ok
}

func (s containsRecursiveSeen) remember(t typ.Type) {
	if s == nil || !containsRecursiveSeenTracks(t) {
		return
	}
	s[t] = struct{}{}
}

func containsRecursiveSeenTracks(t typ.Type) bool {
	switch t.(type) {
	case *typ.Optional,
		*typ.Union,
		*typ.Intersection,
		*typ.Array,
		*typ.Map,
		*typ.ReadonlyMap,
		*typ.Tuple,
		*typ.Function,
		*typ.Record,
		*typ.Alias,
		*typ.Meta,
		*typ.Generic,
		*typ.Instantiated,
		*typ.TypeParam,
		*typ.Interface:
		return true
	default:
		return false
	}
}

func containsRecursive(t typ.Type, seen containsRecursiveSeen) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotations(t)
	if t == nil {
		return false
	}
	if _, ok := t.(*typ.Recursive); ok {
		return true
	}
	if seen.contains(t) {
		return false
	}
	seen.remember(t)

	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return containsRecursive(o.Inner, seen)
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if containsRecursive(member, seen) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if containsRecursive(member, seen) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return containsRecursive(a.Element, seen)
		},
		Map: func(m *typ.Map) bool {
			return containsRecursive(m.Key, seen) ||
				containsRecursive(m.Value, seen)
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) bool {
			return containsRecursive(m.Key, seen) ||
				containsRecursive(m.Value, seen)
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, elem := range tup.Elements {
				if containsRecursive(elem, seen) {
					return true
				}
			}
			return false
		},
		Function: func(fn *typ.Function) bool {
			for _, param := range fn.Params {
				if containsRecursive(param.Type, seen) {
					return true
				}
			}
			if containsRecursive(fn.Variadic, seen) {
				return true
			}
			for _, ret := range fn.Returns {
				if containsRecursive(ret, seen) {
					return true
				}
			}
			for _, param := range fn.TypeParams {
				if param != nil && containsRecursive(param.Constraint, seen) {
					return true
				}
			}
			return false
		},
		Record: func(r *typ.Record) bool {
			if containsRecursive(r.MapKey, seen) ||
				containsRecursive(r.MapValue, seen) ||
				containsRecursive(r.Metatable, seen) {
				return true
			}
			for _, field := range r.Fields {
				if containsRecursive(field.Type, seen) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if containsRecursive(member.Type, seen) {
					return true
				}
			}
			return false
		},
		Alias: func(a *typ.Alias) bool {
			return containsRecursive(a.Target, seen)
		},
		Meta: func(m *typ.Meta) bool {
			return containsRecursive(m.Of, seen)
		},
		Generic: func(g *typ.Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && containsRecursive(param.Constraint, seen) {
					return true
				}
			}
			return containsRecursive(g.Body, seen)
		},
		Instantiated: func(i *typ.Instantiated) bool {
			if i.Generic != nil && containsRecursive(i.Generic, seen) {
				return true
			}
			for _, arg := range i.TypeArgs {
				if containsRecursive(arg, seen) {
					return true
				}
			}
			return false
		},
		TypeParam: func(p *typ.TypeParam) bool {
			return containsRecursive(p.Constraint, seen)
		},
		Interface: func(i *typ.Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && containsRecursive(method.Type, seen) {
					return true
				}
			}
			return false
		},
		Recursive: func(*typ.Recursive) bool {
			return true
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}

type containsSeen map[uint64][]typ.Type

func (s containsSeen) contains(t typ.Type) bool {
	if t == nil || s == nil {
		return false
	}
	for _, existing := range s[containsSeenKey(t)] {
		if typ.TypeEquals(existing, t) {
			return true
		}
	}
	return false
}

func (s containsSeen) remember(t typ.Type) {
	if t == nil || s == nil {
		return
	}
	hash := containsSeenKey(t)
	s[hash] = append(s[hash], t)
}

func containsSeenKey(t typ.Type) uint64 {
	if t == nil {
		return 0
	}
	return typ.EqualityHash(t)
}

func contains(t typ.Type, pred func(typ.Type) bool, seen containsSeen) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(t)
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

	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return contains(o.Inner, pred, seen)
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if contains(member, pred, seen) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if contains(member, pred, seen) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return contains(a.Element, pred, seen)
		},
		Map: func(m *typ.Map) bool {
			return contains(m.Key, pred, seen) ||
				contains(m.Value, pred, seen)
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) bool {
			return contains(m.Key, pred, seen) ||
				contains(m.Value, pred, seen)
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, elem := range tup.Elements {
				if contains(elem, pred, seen) {
					return true
				}
			}
			return false
		},
		Function: func(fn *typ.Function) bool {
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
		Record: func(r *typ.Record) bool {
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
			for _, member := range r.StaticMembers {
				if contains(member.Type, pred, seen) {
					return true
				}
			}
			return false
		},
		Alias: func(a *typ.Alias) bool {
			return contains(a.Target, pred, seen)
		},
		Meta: func(m *typ.Meta) bool {
			return contains(m.Of, pred, seen)
		},
		Generic: func(g *typ.Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && contains(param.Constraint, pred, seen) {
					return true
				}
			}
			return contains(g.Body, pred, seen)
		},
		Instantiated: func(i *typ.Instantiated) bool {
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
		TypeParam: func(p *typ.TypeParam) bool {
			return contains(p.Constraint, pred, seen)
		},
		Interface: func(i *typ.Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && contains(method.Type, pred, seen) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *typ.Recursive) bool {
			return contains(r.Body, pred, seen)
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}
