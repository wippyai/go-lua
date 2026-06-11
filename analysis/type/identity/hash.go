package identity

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// EqualityHash returns the canonical hash used by structural equality and
// deduplication. It matches Hash for immutable closed products, while also
// recomputing through recursive wrappers so stale construction-time hashes do
// not leak into equality policy.
func EqualityHash(t typ.Type) uint64 {
	t = unwrapAliasForEquals(t, typ.NewGuard())
	if t == nil {
		return 0
	}
	return hashBodyWithVisitedMemo(t, make(map[*typ.Recursive]bool), make(map[typ.Type]uint64))
}

func hashWithVisitedMemo(t typ.Type, visited map[*typ.Recursive]bool, memo map[typ.Type]uint64) uint64 {
	if t == nil {
		return 0
	}

	if rec, ok := t.(*typ.Recursive); ok {
		if visited[rec] {
			return hash.MixHash(uint64(kind.Recursive), hash.FnvString("$self"))
		}
		if h, ok := memo[t]; ok {
			return h
		}
		visited[rec] = true
		defer delete(visited, rec)

		h := hash.MixHash(uint64(kind.Recursive), hash.FnvString(rec.Name))
		if rec.Body != nil {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(rec.Body, visited, memo))
		}
		memo[t] = h
		return h
	}

	if h, ok := memo[t]; ok {
		return h
	}
	h := hashBodyWithVisitedMemo(t, visited, memo)
	memo[t] = h
	return h
}

func hashBodyWithVisitedMemo(t typ.Type, visited map[*typ.Recursive]bool, memo map[typ.Type]uint64) uint64 {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = unwrap.Annotated(t)
	if alias, ok := t.(*typ.Alias); ok {
		return hashBodyWithVisitedMemo(alias.UnaliasedTarget(), visited, memo)
	}

	if rec, ok := t.(*typ.Recursive); ok {
		return hashWithVisitedMemo(rec, visited, memo)
	}
	if h, ok := memo[t]; ok {
		return h
	}

	h := typ.Visit(t, typ.Visitor[uint64]{
		Optional: func(o *typ.Optional) uint64 {
			return hash.MixHash(uint64(kind.Optional), hashBodyWithVisitedMemo(o.Inner, visited, memo))
		},
		Union: func(u *typ.Union) uint64 {
			h := uint64(kind.Union)
			for _, m := range u.Members {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(m, visited, memo))
			}
			return h
		},
		Intersection: func(in *typ.Intersection) uint64 {
			h := uint64(kind.Intersection)
			for _, m := range in.Members {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(m, visited, memo))
			}
			return h
		},
		Record: func(r *typ.Record) uint64 {
			h := uint64(kind.Record)
			for _, f := range r.Fields {
				h = hash.MixHash(h, hash.FnvString(f.Name))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(f.Type, visited, memo))
				if f.Optional {
					h = hash.MixHash(h, 1)
				}
				if f.Readonly {
					h = hash.MixHash(h, 2)
				}
			}
			for _, m := range r.StaticMembers {
				h = hash.MixHash(h, recordStaticHash)
				h = hash.MixHash(h, uint64(m.Kind))
				switch m.Kind {
				case typ.StaticMemberStringIndex:
					h = hash.MixHash(h, hash.FnvString(m.Name))
				case typ.StaticMemberIntIndex:
					h = hash.MixHash(h, uint64(m.Index))
				}
				h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Type, visited, memo))
				if m.Optional {
					h = hash.MixHash(h, 1)
				}
				if m.Readonly {
					h = hash.MixHash(h, 2)
				}
			}
			if r.Metatable != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r.Metatable, visited, memo))
			}
			if r.Open {
				h = hash.MixHash(h, 3)
			}
			if r.HasMapComponent() {
				h = hash.MixHash(h, hash.FnvString("$mapKey"))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r.MapKey, visited, memo))
				h = hash.MixHash(h, hash.FnvString("$mapValue"))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r.MapValue, visited, memo))
			}
			return h
		},
		Array: func(a *typ.Array) uint64 {
			return hash.MixHash(uint64(kind.Array), hashBodyWithVisitedMemo(a.Element, visited, memo))
		},
		Map: func(m *typ.Map) uint64 {
			h := uint64(kind.Map)
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Key, visited, memo))
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Value, visited, memo))
			return h
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) uint64 {
			h := uint64(kind.ReadonlyMap)
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Key, visited, memo))
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Value, visited, memo))
			return h
		},
		Tuple: func(tup *typ.Tuple) uint64 {
			h := uint64(kind.Tuple)
			for _, e := range tup.Elements {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(e, visited, memo))
			}
			return h
		},
		Function: func(fn *typ.Function) uint64 {
			h := uint64(kind.Function)
			for _, tp := range fn.TypeParams {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(tp, visited, memo))
			}
			for _, p := range fn.Params {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(p.Type, visited, memo))
				if p.Optional {
					h = hash.MixHash(h, 1)
				}
			}
			if fn.Variadic != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(fn.Variadic, visited, memo))
			}
			for _, r := range fn.Returns {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r, visited, memo))
			}
			return h
		},
		Meta: func(m *typ.Meta) uint64 {
			return hash.MixHash(uint64(kind.Meta), hashBodyWithVisitedMemo(m.Of, visited, memo))
		},
		Generic: func(g *typ.Generic) uint64 {
			h := hash.MixHash(uint64(kind.Generic), hash.FnvString(g.Name))
			for _, p := range g.TypeParams {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(p, visited, memo))
			}
			if g.Name == "" && g.Body != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(g.Body, visited, memo))
			}
			return h
		},
		Instantiated: func(in *typ.Instantiated) uint64 {
			h := hash.MixHash(uint64(kind.Instantiated), hashBodyWithVisitedMemo(in.Generic, visited, memo))
			for _, arg := range in.TypeArgs {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(arg, visited, memo))
			}
			return h
		},
		TypeParam: func(tp *typ.TypeParam) uint64 {
			h := hash.MixHash(uint64(kind.TypeParam), hash.FnvString(tp.Name))
			if tp.Constraint != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(tp.Constraint, visited, memo))
			}
			return h
		},
		Interface: func(i *typ.Interface) uint64 {
			h := hash.MixHash(uint64(kind.Interface), hash.FnvString(i.Name))
			for _, method := range i.Methods {
				h = hash.MixHash(h, hash.FnvString(method.Name))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(method.Type, visited, memo))
			}
			return h
		},
		Default: func(t typ.Type) uint64 {
			return t.Hash()
		},
	})
	memo[t] = h
	return h
}

var recordStaticHash = hash.FnvString("$staticMember")
