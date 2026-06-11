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
	return Contains(t, func(t typ.Type) bool {
		_, ok := t.(*typ.Recursive)
		return ok
	})
}

type containsSeen map[uint64][]typ.Type

func (s containsSeen) contains(t typ.Type) bool {
	if t == nil || s == nil {
		return false
	}
	for _, existing := range s[containsSeenKey(t)] {
		if typeEquals(existing, t) {
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
	return t.Hash()
}

func typeEquals(a, b typ.Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equals(b)
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
