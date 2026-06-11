package refinement

import (
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// NeedsSameExpressionFallback reports whether t contains a leaf that can be
// repaired by a same-expression fallback. This is deliberately broader than
// free type parameters: a summary return may contain unknown/any/deferred leaves
// inside otherwise precise structure, and those holes should be repaired by a
// closed signature observation without replacing the whole value.
func NeedsSameExpressionFallback(t typ.Type) bool {
	scan := &sameExpressionFallbackScan{seen: make(sameExpressionFallbackSeen)}
	return scan.needs(t)
}

// NeedsSameExpressionFallbackWithin is the bounded form of
// NeedsSameExpressionFallback. When maxNodes is positive and the scan exceeds
// it, the returned complete flag is false and the caller should treat this as
// "no precision repair from this optional fallback" rather than as proof that no
// repairable leaf exists.
func NeedsSameExpressionFallbackWithin(t typ.Type, maxNodes int) (needs bool, complete bool) {
	scan := &sameExpressionFallbackScan{seen: make(sameExpressionFallbackSeen), maxNodes: maxNodes}
	needs = scan.needs(t)
	return needs, !scan.exceeded
}

type sameExpressionFallbackSeen map[uintptr]struct{}

func (s sameExpressionFallbackSeen) contains(t typ.Type) bool {
	key := nodeid.Pointer(t)
	if key == 0 || s == nil {
		return false
	}
	_, ok := s[key]
	return ok
}

func (s sameExpressionFallbackSeen) remember(t typ.Type) {
	key := nodeid.Pointer(t)
	if key == 0 || s == nil {
		return
	}
	s[key] = struct{}{}
}

type sameExpressionFallbackScan struct {
	seen     sameExpressionFallbackSeen
	maxNodes int
	nodes    int
	exceeded bool
}

func (s *sameExpressionFallbackScan) enter() bool {
	if s == nil || s.maxNodes <= 0 {
		return true
	}
	s.nodes++
	if s.nodes <= s.maxNodes {
		return true
	}
	s.exceeded = true
	return false
}

func (s *sameExpressionFallbackScan) needs(t typ.Type) bool {
	if !s.enter() {
		return false
	}
	if t == nil {
		return true
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return true
	}
	if summaryNeedsFallbackLeaf(t) {
		return true
	}
	if s.seen.contains(t) {
		return false
	}
	s.seen.remember(t)
	switch v := t.(type) {
	case *typ.Function:
		// Function parameters are contravariant input positions. A loose summary
		// parameter (`any`, unknown, optional self) should not trigger a fallback
		// call by itself because RefineWithFallback preserves such parameters
		// unless the function has an output/covariant hole to repair.
		for _, ret := range v.Returns {
			if s.needs(ret) {
				return true
			}
		}
		return false
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if s.needs(arg) {
				return true
			}
		}
		return false
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return s.needs(o.Inner)
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if s.needs(member) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if s.needs(member) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return s.needs(a.Element)
		},
		Map: func(m *typ.Map) bool {
			return s.needs(m.Key) || s.needs(m.Value)
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) bool {
			return s.needs(m.Key) || s.needs(m.Value)
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, elem := range tup.Elements {
				if s.needs(elem) {
					return true
				}
			}
			return false
		},
		Record: func(r *typ.Record) bool {
			if (r.MapKey != nil && s.needs(r.MapKey)) ||
				(r.MapValue != nil && s.needs(r.MapValue)) ||
				(r.Metatable != nil && s.needs(r.Metatable)) {
				return true
			}
			for _, field := range r.Fields {
				if s.needs(field.Type) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if s.needs(member.Type) {
					return true
				}
			}
			return false
		},
		Alias: func(a *typ.Alias) bool {
			return s.needs(a.Target)
		},
		Meta: func(m *typ.Meta) bool {
			return s.needs(m.Of)
		},
		Recursive: func(r *typ.Recursive) bool {
			return s.needs(r.Body)
		},
	})
}

func summaryNeedsFallbackLeaf(t typ.Type) bool {
	if t == nil || typ.AbsentOrUnknown(t) || t.Kind().IsPlaceholder() || t.Kind().IsDeferred() {
		return true
	}
	_, ok := t.(*typ.TypeParam)
	return ok
}
