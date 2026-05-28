package narrow

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// RefineLengthIndex removes nil from t[#t] when flow proves #t is positive.
// For non-zero offsets, the container must be sequence-shaped because Lua's
// length border only proves the exact border element.
func RefineLengthIndex(container, indexResult typ.Type, lower, offset int64) typ.Type {
	index := lower + offset
	if index < 1 {
		return nil
	}
	if offset == 0 {
		return removeNilWhenOnlyUncertainty(indexResult)
	}
	return RefineSequenceIndex(container, indexResult, index)
}

// RefineSequenceIndex removes nil from an indexed result when flow proves the
// requested positive index is inside a sequence-shaped container.
func RefineSequenceIndex(container, indexResult typ.Type, index int64) typ.Type {
	if !LengthBoundProvesSequenceIndex(container, index) {
		return nil
	}
	return removeNilWhenOnlyUncertainty(indexResult)
}

func removeNilWhenOnlyUncertainty(t typ.Type) typ.Type {
	if !NilPresenceIsOnlyFlowUncertainty(t) {
		return nil
	}
	narrowed := RemoveNil(t)
	if typ.IsNever(narrowed) || typ.TypeEquals(narrowed, t) {
		return nil
	}
	return narrowed
}

// NilPresenceIsOnlyFlowUncertainty reports whether removing nil leaves useful
// value information. It is the guard for presence proofs from flow facts.
func NilPresenceIsOnlyFlowUncertainty(t typ.Type) bool {
	if t == nil {
		return false
	}
	if t.Kind() == kind.Nil {
		return true
	}
	narrowed := RemoveNil(t)
	return !typ.IsNever(narrowed) && !typ.TypeEquals(narrowed, t)
}

// LengthBoundProvesSequenceIndex reports whether a numeric length lower bound
// proves the requested positive index exists for a sequence-shaped type.
func LengthBoundProvesSequenceIndex(t typ.Type, index int64) bool {
	return lengthBoundProvesSequenceIndexDepth(t, index, 0)
}

// TupleArity returns the guaranteed minimum static arity of a fixed-arity tuple
// container. It accepts a bare tuple, or a union/optional whose non-nil members
// are all tuples, returning the minimum element count across those members.
// A non-nil member that is not a tuple, or an empty/zero-arity shape, yields
// ok=false because no positive index can be proven present. The static arity is
// the exact runtime length of a fixed-arity tuple, so #t == arity and any index
// in [1, arity] is provably present.
func TupleArity(t typ.Type) (int64, bool) {
	return tupleArityDepth(t, 0)
}

func tupleArityDepth(t typ.Type, depth int) (int64, bool) {
	if t == nil || typ.DepthExceeded(depth) {
		return 0, false
	}
	t = unwrap.Alias(t)
	if expanded := unwrap.Instantiated(t); expanded != t {
		return tupleArityDepth(expanded, depth+1)
	}
	return typ.Visit(t, typ.Visitor[arityResult]{
		Tuple: func(tuple *typ.Tuple) arityResult {
			if tuple == nil || len(tuple.Elements) == 0 {
				return arityResult{}
			}
			return arityResult{arity: int64(len(tuple.Elements)), ok: true}
		},
		Optional: func(o *typ.Optional) arityResult {
			if o == nil {
				return arityResult{}
			}
			n, ok := tupleArityDepth(o.Inner, depth+1)
			return arityResult{arity: n, ok: ok}
		},
		Union: func(u *typ.Union) arityResult {
			if u == nil {
				return arityResult{}
			}
			min := int64(0)
			seen := false
			for _, m := range u.Members {
				if m == nil || m.Kind() == kind.Nil {
					continue
				}
				n, ok := tupleArityDepth(m, depth+1)
				if !ok {
					return arityResult{}
				}
				if !seen || n < min {
					min = n
				}
				seen = true
			}
			return arityResult{arity: min, ok: seen}
		},
		Default: func(typ.Type) arityResult {
			return arityResult{}
		},
	}).unpack()
}

type arityResult struct {
	arity int64
	ok    bool
}

func (r arityResult) unpack() (int64, bool) { return r.arity, r.ok }

// RefineByLengthLowerBound keeps only values that can satisfy #t >= lower.
// The refinement is intentionally a shape law, not an index-read shortcut:
// positive length eliminates statically empty closed shapes and filters unions,
// while preserving sequence/map shapes whose concrete runtime length is not
// represented by the type lattice.
func RefineByLengthLowerBound(t typ.Type, lower int64) typ.Type {
	if t == nil || lower <= 0 {
		return t
	}
	return refineByLengthLowerBoundDepth(t, lower, 0)
}

func refineByLengthLowerBoundDepth(t typ.Type, lower int64, depth int) typ.Type {
	if t == nil || typ.DepthExceeded(depth) {
		return t
	}
	if typ.IsNever(t) {
		return t
	}
	t = unwrap.Alias(t)
	if expanded := unwrap.Instantiated(t); expanded != t {
		return refineByLengthLowerBoundDepth(expanded, lower, depth+1)
	}
	return typ.Visit(t, typ.Visitor[typ.Type]{
		Optional: func(o *typ.Optional) typ.Type {
			if o == nil {
				return typ.Never
			}
			return refineByLengthLowerBoundDepth(o.Inner, lower, depth+1)
		},
		Union: func(u *typ.Union) typ.Type {
			if u == nil {
				return typ.Never
			}
			members := make([]typ.Type, 0, len(u.Members))
			for _, member := range u.Members {
				refined := refineByLengthLowerBoundDepth(member, lower, depth+1)
				if refined == nil || typ.IsNever(refined) {
					continue
				}
				members = append(members, refined)
			}
			if len(members) == 0 {
				return typ.Never
			}
			if len(members) == 1 {
				return members[0]
			}
			return typ.NewUnion(members...)
		},
		Intersection: func(in *typ.Intersection) typ.Type {
			if in == nil {
				return typ.Never
			}
			members := make([]typ.Type, 0, len(in.Members))
			changed := false
			for _, member := range in.Members {
				refined := refineByLengthLowerBoundDepth(member, lower, depth+1)
				if refined == nil || typ.IsNever(refined) {
					return typ.Never
				}
				if !typ.TypeEquals(refined, member) {
					changed = true
				}
				members = append(members, refined)
			}
			if !changed {
				return t
			}
			return typ.NewIntersection(members...)
		},
		Array: func(*typ.Array) typ.Type {
			return t
		},
		Tuple: func(tuple *typ.Tuple) typ.Type {
			if tuple != nil && int64(len(tuple.Elements)) >= lower {
				return t
			}
			return typ.Never
		},
		Map: func(m *typ.Map) typ.Type {
			if m != nil && typeMayContainSequenceKey(m.Key, depth+1) {
				return t
			}
			return typ.Never
		},
		Record: func(rec *typ.Record) typ.Type {
			if recordMayHaveLengthAtLeast(rec, lower, depth+1) {
				return t
			}
			return typ.Never
		},
		Recursive: func(r *typ.Recursive) typ.Type {
			if r == nil || r.Body == nil || r.Body == r {
				return t
			}
			refined := refineByLengthLowerBoundDepth(r.Body, lower, depth+1)
			if refined == nil || typ.IsNever(refined) {
				return typ.Never
			}
			return t
		},
		Literal: func(lit *typ.Literal) typ.Type {
			if lit == nil {
				return typ.Never
			}
			if lit.Base == kind.String {
				if s, ok := lit.Value.(string); ok && int64(len(s)) >= lower {
					return t
				}
				return typ.Never
			}
			return t
		},
		Default: func(t typ.Type) typ.Type {
			if t != nil && t.Kind() == kind.String {
				return t
			}
			return t
		},
	})
}

func recordMayHaveLengthAtLeast(rec *typ.Record, lower int64, depth int) bool {
	if rec == nil {
		return false
	}
	if lower <= 0 {
		return true
	}
	// A fresh empty-table seed ({}) is the gradual bottom of the table lattice:
	// it can still grow into a sequence (table.insert widens it), so a positive
	// length lower bound is compatible with it rather than a contradiction.
	if rec.Fresh {
		return true
	}
	if rec.Open || rec.Metatable != nil {
		return true
	}
	return rec.HasMapComponent() && typeMayContainSequenceKey(rec.MapKey, depth+1)
}

func typeMayContainSequenceKey(t typ.Type, depth int) bool {
	if t == nil || typ.DepthExceeded(depth) {
		return true
	}
	t = unwrap.Alias(t)
	if expanded := unwrap.Instantiated(t); expanded != t {
		return typeMayContainSequenceKey(expanded, depth+1)
	}
	if t.Kind().IsPlaceholder() {
		return true
	}
	return TypesOverlap(t, typ.Integer)
}

func lengthBoundProvesSequenceIndexDepth(t typ.Type, index int64, depth int) bool {
	if t == nil || typ.DepthExceeded(depth) {
		return false
	}
	t = unwrap.Alias(t)
	if expanded := unwrap.Instantiated(t); expanded != t {
		return lengthBoundProvesSequenceIndexDepth(expanded, index, depth+1)
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Array: func(*typ.Array) bool {
			return true
		},
		Tuple: func(tuple *typ.Tuple) bool {
			return tuple != nil && int64(len(tuple.Elements)) >= index
		},
		Optional: func(o *typ.Optional) bool {
			return lengthBoundProvesSequenceIndexDepth(o.Inner, index, depth+1)
		},
		Union: func(u *typ.Union) bool {
			found := false
			for _, m := range u.Members {
				if m == nil || m.Kind() == kind.Nil {
					continue
				}
				if lengthBoundProvesSequenceIndexDepth(m, index, depth+1) {
					found = true
					continue
				}
				if typeMaxLenLessThanIndex(m, index, depth+1) {
					continue
				}
				return false
			}
			return found
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if lengthBoundProvesSequenceIndexDepth(m, index, depth+1) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *typ.Recursive) bool {
			if r.Body == nil || r.Body == r {
				return false
			}
			return lengthBoundProvesSequenceIndexDepth(r.Body, index, depth+1)
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}

func typeMaxLenLessThanIndex(t typ.Type, index int64, depth int) bool {
	if t == nil || typ.DepthExceeded(depth) {
		return false
	}
	t = unwrap.Alias(t)
	if expanded := unwrap.Instantiated(t); expanded != t {
		return typeMaxLenLessThanIndex(expanded, index, depth+1)
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Tuple: func(tuple *typ.Tuple) bool {
			return tuple == nil || int64(len(tuple.Elements)) < index
		},
		Record: func(rec *typ.Record) bool {
			return rec != nil &&
				!rec.Open &&
				!rec.HasMapComponent() &&
				rec.Metatable == nil &&
				index > 0
		},
		Optional: func(o *typ.Optional) bool {
			return o == nil || typeMaxLenLessThanIndex(o.Inner, index, depth+1)
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if m == nil || m.Kind() == kind.Nil {
					continue
				}
				if !typeMaxLenLessThanIndex(m, index, depth+1) {
					return false
				}
			}
			return true
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if typeMaxLenLessThanIndex(m, index, depth+1) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *typ.Recursive) bool {
			if r.Body == nil || r.Body == r {
				return false
			}
			return typeMaxLenLessThanIndex(r.Body, index, depth+1)
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}
