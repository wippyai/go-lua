package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

func hashWithVisitedMemo(t Type, visited map[*Recursive]bool, memo map[Type]uint64) uint64 {
	if t == nil {
		return 0
	}

	// Check if this is a recursive type we've already seen
	if rec, ok := t.(*Recursive); ok {
		if visited[rec] {
			// Self-reference: use a sentinel hash value
			return hash.MixHash(uint64(kind.Recursive), hash.FnvString("$self"))
		}
		if h, ok := memo[t]; ok {
			return h
		}
		visited[rec] = true
		defer delete(visited, rec)

		// Compute structurally rather than using pre-computed hash.
		// This ensures correct hashing during mutual recursion setup
		// when the other recursive type's hash may not be computed yet.
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

func hashBodyWithVisitedMemo(t Type, visited map[*Recursive]bool, memo map[Type]uint64) uint64 {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = unwrapTransparentWrappers(t)
	if alias, ok := t.(*Alias); ok {
		return hashBodyWithVisitedMemo(alias.UnaliasedTarget(), visited, memo)
	}

	// Check for recursive type reference
	if rec, ok := t.(*Recursive); ok {
		return hashWithVisitedMemo(rec, visited, memo)
	}

	if h, ok := memo[t]; ok {
		return h
	}

	// For compound types, traverse their components
	h := Visit(t, Visitor[uint64]{
		Optional: func(o *Optional) uint64 {
			return hash.MixHash(uint64(kind.Optional), hashBodyWithVisitedMemo(o.Inner, visited, memo))
		},
		Union: func(u *Union) uint64 {
			h := uint64(kind.Union)
			for _, m := range u.Members {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(m, visited, memo))
			}
			return h
		},
		Intersection: func(in *Intersection) uint64 {
			h := uint64(kind.Intersection)
			for _, m := range in.Members {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(m, visited, memo))
			}
			return h
		},
		Record: func(r *Record) uint64 {
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
				case StaticMemberStringIndex:
					h = hash.MixHash(h, hash.FnvString(m.Name))
				case StaticMemberIntIndex:
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
		Array: func(a *Array) uint64 {
			return hash.MixHash(uint64(kind.Array), hashBodyWithVisitedMemo(a.Element, visited, memo))
		},
		Map: func(m *Map) uint64 {
			h := uint64(kind.Map)
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Key, visited, memo))
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Value, visited, memo))
			return h
		},
		ReadonlyMap: func(m *ReadonlyMap) uint64 {
			h := uint64(kind.ReadonlyMap)
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Key, visited, memo))
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Value, visited, memo))
			return h
		},
		Tuple: func(t *Tuple) uint64 {
			h := uint64(kind.Tuple)
			for _, e := range t.Elements {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(e, visited, memo))
			}
			return h
		},
		Function: func(fn *Function) uint64 {
			h := uint64(kind.Function)
			// Type parameters
			for _, tp := range fn.TypeParams {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(tp, visited, memo))
			}
			// Parameters with optional flags
			for _, p := range fn.Params {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(p.Type, visited, memo))
				if p.Optional {
					h = hash.MixHash(h, 1)
				}
			}
			// Variadic
			if fn.Variadic != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(fn.Variadic, visited, memo))
			}
			// Returns
			for _, r := range fn.Returns {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r, visited, memo))
			}
			return h
		},
		Meta: func(m *Meta) uint64 {
			return hash.MixHash(uint64(kind.Meta), hashBodyWithVisitedMemo(m.Of, visited, memo))
		},
		Generic: func(g *Generic) uint64 {
			h := hash.MixHash(uint64(kind.Generic), hash.FnvString(g.Name))
			for _, p := range g.TypeParams {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(p, visited, memo))
			}
			if g.Body != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(g.Body, visited, memo))
			}
			return h
		},
		Instantiated: func(in *Instantiated) uint64 {
			h := hash.MixHash(uint64(kind.Instantiated), hashBodyWithVisitedMemo(in.Generic, visited, memo))
			for _, arg := range in.TypeArgs {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(arg, visited, memo))
			}
			return h
		},
		TypeParam: func(tp *TypeParam) uint64 {
			h := hash.MixHash(uint64(kind.TypeParam), hash.FnvString(tp.Name))
			if tp.Constraint != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(tp.Constraint, visited, memo))
			}
			return h
		},
		Interface: func(i *Interface) uint64 {
			h := hash.MixHash(uint64(kind.Interface), hash.FnvString(i.Name))
			for _, method := range i.Methods {
				h = hash.MixHash(h, hash.FnvString(method.Name))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(method.Type, visited, memo))
			}
			return h
		},
		Default: func(t Type) uint64 {
			return t.Hash()
		},
	})
	memo[t] = h
	return h
}

// EqualityHash returns the canonical hash used by structural equality and
// deduplication. It matches Hash for immutable closed products, but recomputes
// wrappers around open recursive placeholders so SetBody cannot leave stale
// construction-time hashes in the type algebra.
func EqualityHash(t Type) uint64 {
	t = unwrapAliasForEquals(t, NewGuard())
	if t == nil {
		return 0
	}
	if knownContainsOpenRecursive(t) {
		return hashBodyWithVisitedMemo(t, make(map[*Recursive]bool), make(map[Type]uint64))
	}
	return t.Hash()
}

func equalityHash(t Type) uint64 {
	return EqualityHash(t)
}

func typeEqualityHash(t Type) uint64 {
	return EqualityHash(t)
}

func (r *Recursive) Hash() uint64 {
	if r.hash != 0 && recursiveHashDepsValid(r.hashDeps) {
		return r.hash
	}
	// Compute hash on demand with cycle detection. Recursive types are mutable
	// only until SetBody completes, then share the same cached-hash contract as
	// other type nodes.
	h := hashWithVisitedMemo(r, make(map[*Recursive]bool), make(map[Type]uint64))
	if deps, ok := recursiveHashDeps(r); ok {
		r.hash = h
		r.hashDeps = deps
	}
	return h
}
