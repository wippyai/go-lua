package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

const (
	recursiveHashSmallVisitedCap = 8
	recursiveHashSmallMemoCap    = 16
)

type recursiveHashMemoEntry struct {
	t Type
	h uint64
}

type recursiveHashScratch struct {
	visited    [recursiveHashSmallVisitedCap]*Recursive
	visitedLen int
	visitedMap map[*Recursive]bool

	memo    [recursiveHashSmallMemoCap]recursiveHashMemoEntry
	memoLen int
	memoMap map[Type]uint64
}

func (s *recursiveHashScratch) visitedContains(r *Recursive) bool {
	if s.visitedMap != nil {
		return s.visitedMap[r]
	}
	for i := 0; i < s.visitedLen; i++ {
		if s.visited[i] == r {
			return true
		}
	}
	return false
}

func (s *recursiveHashScratch) visitedPush(r *Recursive) {
	if s.visitedMap != nil {
		s.visitedMap[r] = true
		return
	}
	if s.visitedLen < len(s.visited) {
		s.visited[s.visitedLen] = r
		s.visitedLen++
		return
	}
	s.visitedMap = make(map[*Recursive]bool, len(s.visited)+1)
	for i := 0; i < s.visitedLen; i++ {
		s.visitedMap[s.visited[i]] = true
	}
	s.visitedMap[r] = true
}

func (s *recursiveHashScratch) visitedPop(r *Recursive) {
	if s.visitedMap != nil {
		delete(s.visitedMap, r)
		return
	}
	if s.visitedLen == 0 {
		return
	}
	s.visitedLen--
	s.visited[s.visitedLen] = nil
}

func (s *recursiveHashScratch) memoGet(t Type) (uint64, bool) {
	if s.memoMap != nil {
		h, ok := s.memoMap[t]
		return h, ok
	}
	for i := 0; i < s.memoLen; i++ {
		if s.memo[i].t == t {
			return s.memo[i].h, true
		}
	}
	return 0, false
}

func (s *recursiveHashScratch) memoSet(t Type, h uint64) {
	if s.memoMap != nil {
		s.memoMap[t] = h
		return
	}
	for i := 0; i < s.memoLen; i++ {
		if s.memo[i].t == t {
			s.memo[i].h = h
			return
		}
	}
	if s.memoLen < len(s.memo) {
		s.memo[s.memoLen] = recursiveHashMemoEntry{t: t, h: h}
		s.memoLen++
		return
	}
	s.memoMap = make(map[Type]uint64, len(s.memo)+1)
	for i := 0; i < s.memoLen; i++ {
		s.memoMap[s.memo[i].t] = s.memo[i].h
	}
	s.memoMap[t] = h
}

func hashWithVisitedMemo(t Type, scratch *recursiveHashScratch) uint64 {
	if t == nil {
		return 0
	}

	// Check if this is a recursive type we've already seen
	if rec, ok := t.(*Recursive); ok {
		if scratch.visitedContains(rec) {
			// Self-reference: use a sentinel hash value
			return hash.MixHash(uint64(kind.Recursive), hash.FnvString("$self"))
		}
		if h, ok := scratch.memoGet(t); ok {
			return h
		}
		scratch.visitedPush(rec)
		defer scratch.visitedPop(rec)

		// Compute structurally rather than using pre-computed hash.
		// This ensures correct hashing during mutual recursion setup
		// when the other recursive type's hash may not be computed yet.
		h := hash.MixHash(uint64(kind.Recursive), hash.FnvString(rec.Name))
		if rec.Body != nil {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(rec.Body, scratch))
		}
		scratch.memoSet(t, h)
		return h
	}

	if h, ok := scratch.memoGet(t); ok {
		return h
	}
	h := hashBodyWithVisitedMemo(t, scratch)
	scratch.memoSet(t, h)
	return h
}

func hashBodyWithVisitedMemo(t Type, scratch *recursiveHashScratch) uint64 {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = unwrapTransparentWrappers(t)
	if alias, ok := t.(*Alias); ok {
		return hashBodyWithVisitedMemo(alias.UnaliasedTarget(), scratch)
	}

	// Check for recursive type reference
	if rec, ok := t.(*Recursive); ok {
		return hashWithVisitedMemo(rec, scratch)
	}

	if h, ok := scratch.memoGet(t); ok {
		return h
	}

	// For compound types, traverse their components
	h := Visit(t, Visitor[uint64]{
		Optional: func(o *Optional) uint64 {
			return hash.MixHash(uint64(kind.Optional), hashBodyWithVisitedMemo(o.Inner, scratch))
		},
		Union: func(u *Union) uint64 {
			h := uint64(kind.Union)
			for _, m := range u.Members {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(m, scratch))
			}
			return h
		},
		Intersection: func(in *Intersection) uint64 {
			h := uint64(kind.Intersection)
			for _, m := range in.Members {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(m, scratch))
			}
			return h
		},
		Record: func(r *Record) uint64 {
			h := uint64(kind.Record)
			for _, f := range r.Fields {
				h = hash.MixHash(h, hash.FnvString(f.Name))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(f.Type, scratch))
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
				h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Type, scratch))
				if m.Optional {
					h = hash.MixHash(h, 1)
				}
				if m.Readonly {
					h = hash.MixHash(h, 2)
				}
			}
			if r.Metatable != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r.Metatable, scratch))
			}
			if r.Open {
				h = hash.MixHash(h, 3)
			}
			if r.HasMapComponent() {
				h = hash.MixHash(h, hash.FnvString("$mapKey"))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r.MapKey, scratch))
				h = hash.MixHash(h, hash.FnvString("$mapValue"))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r.MapValue, scratch))
			}
			return h
		},
		Array: func(a *Array) uint64 {
			return hash.MixHash(uint64(kind.Array), hashBodyWithVisitedMemo(a.Element, scratch))
		},
		Map: func(m *Map) uint64 {
			h := uint64(kind.Map)
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Key, scratch))
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Value, scratch))
			return h
		},
		ReadonlyMap: func(m *ReadonlyMap) uint64 {
			h := uint64(kind.ReadonlyMap)
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Key, scratch))
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m.Value, scratch))
			return h
		},
		Tuple: func(t *Tuple) uint64 {
			h := uint64(kind.Tuple)
			for _, e := range t.Elements {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(e, scratch))
			}
			return h
		},
		Function: func(fn *Function) uint64 {
			h := uint64(kind.Function)
			// Type parameters
			for _, tp := range fn.TypeParams {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(tp, scratch))
			}
			// Parameters with optional flags
			for _, p := range fn.Params {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(p.Type, scratch))
				if p.Optional {
					h = hash.MixHash(h, 1)
				}
			}
			// Variadic
			if fn.Variadic != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(fn.Variadic, scratch))
			}
			// Returns
			for _, r := range fn.Returns {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(r, scratch))
			}
			return h
		},
		Meta: func(m *Meta) uint64 {
			return hash.MixHash(uint64(kind.Meta), hashBodyWithVisitedMemo(m.Of, scratch))
		},
		Generic: func(g *Generic) uint64 {
			h := hash.MixHash(uint64(kind.Generic), hash.FnvString(g.Name))
			for _, p := range g.TypeParams {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(p, scratch))
			}
			if g.Body != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(g.Body, scratch))
			}
			return h
		},
		Instantiated: func(in *Instantiated) uint64 {
			h := hash.MixHash(uint64(kind.Instantiated), hashBodyWithVisitedMemo(in.Generic, scratch))
			for _, arg := range in.TypeArgs {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(arg, scratch))
			}
			return h
		},
		TypeParam: func(tp *TypeParam) uint64 {
			h := hash.MixHash(uint64(kind.TypeParam), hash.FnvString(tp.Name))
			if tp.Constraint != nil {
				h = hash.MixHash(h, hashBodyWithVisitedMemo(tp.Constraint, scratch))
			}
			return h
		},
		Interface: func(i *Interface) uint64 {
			h := hash.MixHash(uint64(kind.Interface), hash.FnvString(i.Name))
			for _, method := range i.Methods {
				h = hash.MixHash(h, hash.FnvString(method.Name))
				h = hash.MixHash(h, hashBodyWithVisitedMemo(method.Type, scratch))
			}
			return h
		},
		Default: func(t Type) uint64 {
			return t.Hash()
		},
	})
	scratch.memoSet(t, h)
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
		var scratch recursiveHashScratch
		return hashBodyWithVisitedMemo(t, &scratch)
	}
	return t.Hash()
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
	var scratch recursiveHashScratch
	h := hashWithVisitedMemo(r, &scratch)
	if deps, ok := recursiveHashDeps(r); ok {
		r.hash = h
		r.hashDeps = deps
	}
	return h
}
