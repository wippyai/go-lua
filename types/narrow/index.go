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
