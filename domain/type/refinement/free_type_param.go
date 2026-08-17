package refinement

import (
	"github.com/wippyai/go-lua/domain/type/internal/nodeid"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

func (s *freeTypeParamSeen) key(t typ.Type) uint64 {
	if s.containsRecursive(t) {
		if ptr := nodeid.Pointer(t); ptr != 0 {
			return uint64(ptr)
		}
	}
	return typ.EqualityHash(t)
}

// ContainsFreeTypeParam reports whether t contains an unbound symbolic type
// parameter/reference. Unlike ContainsTypeParam, a closed instantiated generic
// such as Box<string> is treated as closed: only its concrete type arguments are
// inspected, not the generic declaration template.
func ContainsFreeTypeParam(t typ.Type) bool {
	var seen freeTypeParamSeen
	return containsFreeTypeParam(t, &seen, nil)
}

type freeTypeParamSeen struct {
	small              [64]freeTypeParamSeenEntry
	smallLen           int
	entries            map[uint64][]typ.Type
	recursiveSeen      freeTypeParamRecursiveSeen
	recursiveMemo      map[typ.Type]bool
	recursiveMemoSmall [8]freeTypeParamRecursiveMemoEntry
	recursiveMemoLen   int
}

type freeTypeParamSeenEntry struct {
	key uint64
	t   typ.Type
}

type freeTypeParamRecursiveMemoEntry struct {
	t      typ.Type
	result bool
}

type freeTypeParamRecursiveSeen struct {
	small    [8]typ.Type
	smallLen int
	entries  map[typ.Type]struct{}
}

func (s *freeTypeParamSeen) enter(t typ.Type) bool {
	if s == nil || t == nil {
		return true
	}
	key := s.key(t)
	for i := 0; i < s.smallLen; i++ {
		entry := s.small[i]
		if entry.key == key && typ.TypeEquals(entry.t, t) {
			return false
		}
	}
	for _, existing := range s.entries[key] {
		if typ.TypeEquals(existing, t) {
			return false
		}
	}
	if s.entries == nil && s.smallLen < len(s.small) {
		s.small[s.smallLen] = freeTypeParamSeenEntry{key: key, t: t}
		s.smallLen++
		return true
	}
	if s.entries == nil {
		s.entries = make(map[uint64][]typ.Type, len(s.small)+1)
		for i := 0; i < s.smallLen; i++ {
			entry := s.small[i]
			s.entries[entry.key] = append(s.entries[entry.key], entry.t)
			s.small[i] = freeTypeParamSeenEntry{}
		}
		s.smallLen = 0
	}
	s.entries[key] = append(s.entries[key], t)
	return true
}

func (s *freeTypeParamSeen) containsRecursive(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.ContainsRecursive(t) {
		return true
	}
	if !typ.ContainsGeneric(t) {
		return false
	}
	if result, ok := s.recursiveMemoLookup(t); ok {
		return result
	}
	s.recursiveSeen.clear()
	result := s.containsRecursiveScan(t)
	s.recursiveMemoRemember(t, result)
	return result
}

func (s *freeTypeParamSeen) containsRecursiveScan(t typ.Type) bool {
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
	if !s.recursiveSeen.enter(t) {
		return false
	}
	return typ.WalkChildren(t, func(child typ.Type) bool {
		return s.containsRecursiveScan(child)
	})
}

func (s *freeTypeParamSeen) recursiveMemoLookup(t typ.Type) (bool, bool) {
	for i := 0; i < s.recursiveMemoLen; i++ {
		entry := s.recursiveMemoSmall[i]
		if entry.t == t {
			return entry.result, true
		}
	}
	if s.recursiveMemo == nil {
		return false, false
	}
	result, ok := s.recursiveMemo[t]
	return result, ok
}

func (s *freeTypeParamSeen) recursiveMemoRemember(t typ.Type, result bool) {
	if s.recursiveMemo == nil && s.recursiveMemoLen < len(s.recursiveMemoSmall) {
		s.recursiveMemoSmall[s.recursiveMemoLen] = freeTypeParamRecursiveMemoEntry{t: t, result: result}
		s.recursiveMemoLen++
		return
	}
	if s.recursiveMemo == nil {
		s.recursiveMemo = make(map[typ.Type]bool, len(s.recursiveMemoSmall)+1)
		for i := 0; i < s.recursiveMemoLen; i++ {
			entry := s.recursiveMemoSmall[i]
			s.recursiveMemo[entry.t] = entry.result
			s.recursiveMemoSmall[i] = freeTypeParamRecursiveMemoEntry{}
		}
		s.recursiveMemoLen = 0
	}
	s.recursiveMemo[t] = result
}

func (s *freeTypeParamRecursiveSeen) enter(t typ.Type) bool {
	if t == nil || !freeTypeParamRecursiveSeenTracks(t) {
		return true
	}
	for i := 0; i < s.smallLen; i++ {
		if s.small[i] == t {
			return false
		}
	}
	if _, ok := s.entries[t]; ok {
		return false
	}
	if s.entries == nil && s.smallLen < len(s.small) {
		s.small[s.smallLen] = t
		s.smallLen++
		return true
	}
	if s.entries == nil {
		s.entries = make(map[typ.Type]struct{}, len(s.small)+1)
		for i := 0; i < s.smallLen; i++ {
			s.entries[s.small[i]] = struct{}{}
			s.small[i] = nil
		}
		s.smallLen = 0
	}
	s.entries[t] = struct{}{}
	return true
}

func (s *freeTypeParamRecursiveSeen) clear() {
	for i := 0; i < s.smallLen; i++ {
		s.small[i] = nil
	}
	s.smallLen = 0
	if s.entries != nil {
		clear(s.entries)
	}
}

func freeTypeParamRecursiveSeenTracks(t typ.Type) bool {
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

func containsFreeTypeParam(t typ.Type, seen *freeTypeParamSeen, owned map[*typ.TypeParam]int) bool {
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

	if freeTypeParamUseSeen(t, owned) {
		if !seen.enter(t) {
			return false
		}
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
	case *typ.Interface:
		return false
	}

	return typ.WalkChildren(t, func(child typ.Type) bool {
		return containsFreeTypeParam(child, seen, owned)
	})
}

func freeTypeParamUseSeen(t typ.Type, owned map[*typ.TypeParam]int) bool {
	return len(owned) == 0 && typ.ContainsRecursive(t)
}
