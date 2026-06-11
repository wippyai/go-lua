package refinement

import (
	"github.com/wippyai/go-lua/analysis/type/identity"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type containsSeen map[uint64][]typ.Type

func (s containsSeen) contains(t typ.Type) bool {
	if t == nil || s == nil {
		return false
	}
	for _, existing := range s[containsSeenKey(t)] {
		if identity.TypeEquals(existing, t) {
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
	if inspect.ContainsRecursive(t) {
		if ptr := nodeid.Pointer(t); ptr != 0 {
			return uint64(ptr)
		}
	}
	return identity.EqualityHash(t)
}

// ContainsFreeTypeParam reports whether t contains an unbound symbolic type
// parameter/reference. Unlike ContainsTypeParam, a closed instantiated generic
// such as Box<string> is treated as closed: only its concrete type arguments are
// inspected, not the generic declaration template.
func ContainsFreeTypeParam(t typ.Type) bool {
	return containsFreeTypeParam(t, make(containsSeen), nil)
}

func containsFreeTypeParam(t typ.Type, seen containsSeen, owned map[*typ.TypeParam]int) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return false
	}

	switch v := t.(type) {
	case *typ.TypeParam:
		return owned == nil || owned[v] == 0
	}

	switch t.Kind() {
	case kind.Ref, kind.Generic:
		return true
	}

	if freeTypeParamUseSeen(owned) {
		if seen.contains(t) {
			return false
		}
		seen.remember(t)
	}

	switch v := t.(type) {
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if containsFreeTypeParam(arg, seen, owned) {
				return true
			}
		}
		return false
	case *typ.Function:
		nextOwned := owned
		if len(v.TypeParams) > 0 {
			nextOwned = make(map[*typ.TypeParam]int, len(owned)+len(v.TypeParams))
			for tp, count := range owned {
				nextOwned[tp] = count
			}
			for _, tp := range v.TypeParams {
				if tp != nil {
					nextOwned[tp]++
				}
			}
		}
		for _, param := range v.Params {
			if containsFreeTypeParam(param.Type, seen, nextOwned) {
				return true
			}
		}
		if containsFreeTypeParam(v.Variadic, seen, nextOwned) {
			return true
		}
		for _, ret := range v.Returns {
			if containsFreeTypeParam(ret, seen, nextOwned) {
				return true
			}
		}
		return false
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return containsFreeTypeParam(o.Inner, seen, owned)
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if containsFreeTypeParam(member, seen, owned) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if containsFreeTypeParam(member, seen, owned) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return containsFreeTypeParam(a.Element, seen, owned)
		},
		Map: func(m *typ.Map) bool {
			return containsFreeTypeParam(m.Key, seen, owned) || containsFreeTypeParam(m.Value, seen, owned)
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) bool {
			return containsFreeTypeParam(m.Key, seen, owned) || containsFreeTypeParam(m.Value, seen, owned)
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, elem := range tup.Elements {
				if containsFreeTypeParam(elem, seen, owned) {
					return true
				}
			}
			return false
		},
		Record: func(r *typ.Record) bool {
			if containsFreeTypeParam(r.MapKey, seen, owned) ||
				containsFreeTypeParam(r.MapValue, seen, owned) ||
				containsFreeTypeParam(r.Metatable, seen, owned) {
				return true
			}
			for _, field := range r.Fields {
				if containsFreeTypeParam(field.Type, seen, owned) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if containsFreeTypeParam(member.Type, seen, owned) {
					return true
				}
			}
			return false
		},
		Alias: func(a *typ.Alias) bool {
			return containsFreeTypeParam(a.Target, seen, owned)
		},
		Meta: func(m *typ.Meta) bool {
			return containsFreeTypeParam(m.Of, seen, owned)
		},
		Recursive: func(r *typ.Recursive) bool {
			return containsFreeTypeParam(r.Body, seen, owned)
		},
	})
}

func freeTypeParamUseSeen(owned map[*typ.TypeParam]int) bool {
	return len(owned) == 0
}
