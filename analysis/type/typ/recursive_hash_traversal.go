package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

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
	t = NormalizeNil(t)
	if t == nil {
		return 0
	}
	t = UnwrapTransparentWrappers(t)
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
	if scratch.activeContains(t) {
		return hash.MixHash(uint64(t.Kind()), hash.FnvString("$cycle"))
	}
	scratch.activePush(t)
	defer scratch.activePop(t)

	h := hashBodyNodeWithVisitedMemo(t, scratch)
	scratch.memoSet(t, h)
	return h
}

func hashBodyNodeWithVisitedMemo(t Type, scratch *recursiveHashScratch) uint64 {
	switch tt := t.(type) {
	case *Optional:
		return hash.MixHash(uint64(kind.Optional), hashBodyWithVisitedMemo(tt.Inner, scratch))
	case *Union:
		h := uint64(kind.Union)
		for _, m := range tt.Members {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m, scratch))
		}
		return h
	case *Intersection:
		h := uint64(kind.Intersection)
		for _, m := range tt.Members {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(m, scratch))
		}
		return h
	case *Record:
		return hashRecordWithVisitedMemo(tt, scratch)
	case *Array:
		return hash.MixHash(uint64(kind.Array), hashBodyWithVisitedMemo(tt.Element, scratch))
	case *Map:
		h := uint64(kind.Map)
		h = hash.MixHash(h, hashBodyWithVisitedMemo(tt.Key, scratch))
		h = hash.MixHash(h, hashBodyWithVisitedMemo(tt.Value, scratch))
		return h
	case *ReadonlyMap:
		h := uint64(kind.ReadonlyMap)
		h = hash.MixHash(h, hashBodyWithVisitedMemo(tt.Key, scratch))
		h = hash.MixHash(h, hashBodyWithVisitedMemo(tt.Value, scratch))
		return h
	case *Tuple:
		h := uint64(kind.Tuple)
		for _, e := range tt.Elements {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(e, scratch))
		}
		return h
	case *Function:
		return hashFunctionWithVisitedMemo(tt, scratch)
	case *Meta:
		return hash.MixHash(uint64(kind.Meta), hashBodyWithVisitedMemo(tt.Of, scratch))
	case *Generic:
		h := hash.MixHash(uint64(kind.Generic), hash.FnvString(tt.Name))
		for _, p := range tt.TypeParams {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(p, scratch))
		}
		if tt.Body != nil {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(tt.Body, scratch))
		}
		return h
	case *Instantiated:
		h := hash.MixHash(uint64(kind.Instantiated), hashBodyWithVisitedMemo(tt.Generic, scratch))
		for _, arg := range tt.TypeArgs {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(arg, scratch))
		}
		return h
	case *TypeParam:
		h := hash.MixHash(uint64(kind.TypeParam), hash.FnvString(tt.Name))
		if tt.Constraint != nil {
			h = hash.MixHash(h, hashBodyWithVisitedMemo(tt.Constraint, scratch))
		}
		return h
	case *Interface:
		h := hash.MixHash(uint64(kind.Interface), hash.FnvString(tt.Name))
		for _, method := range tt.Methods {
			h = hash.MixHash(h, hash.FnvString(method.Name))
			h = hash.MixHash(h, hashBodyWithVisitedMemo(method.Type, scratch))
		}
		return h
	default:
		return t.Hash()
	}
}

func hashRecordWithVisitedMemo(r *Record, scratch *recursiveHashScratch) uint64 {
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
}

func hashFunctionWithVisitedMemo(fn *Function, scratch *recursiveHashScratch) uint64 {
	h := uint64(kind.Function)
	for _, tp := range fn.TypeParams {
		h = hash.MixHash(h, hashBodyWithVisitedMemo(tp, scratch))
	}
	for _, p := range fn.Params {
		h = hash.MixHash(h, hashBodyWithVisitedMemo(p.Type, scratch))
		if p.Optional {
			h = hash.MixHash(h, 1)
		}
	}
	if fn.Variadic != nil {
		h = hash.MixHash(h, hashBodyWithVisitedMemo(fn.Variadic, scratch))
	}
	for _, r := range fn.Returns {
		h = hash.MixHash(h, hashBodyWithVisitedMemo(r, scratch))
	}
	return h
}
