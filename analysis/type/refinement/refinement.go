package refinement

import (
	"github.com/wippyai/go-lua/analysis/type/identity"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type typePair struct {
	a uintptr
	b uintptr
}

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

// MorePreciseFunc reports whether candidate is strictly more precise than baseline.
type MorePreciseFunc func(candidate, baseline typ.Type) bool

// RefineWithFallback merges two same-expression observations without choosing
// one whole value over the other. The summary observation wins where it already
// carries concrete evidence (for example string literals); the fallback repairs
// only top-like or open type-parameter leaves in the same structural position.
func RefineWithFallback(summary, fallback typ.Type, morePrecise MorePreciseFunc) (typ.Type, bool) {
	state := &fallbackRefineState{morePrecise: morePrecise}
	out, changed := state.refine(summary, fallback)
	if !changed {
		return summary, false
	}
	return out, true
}

type fallbackRefineState struct {
	seen        map[typePair]bool
	ownedType   map[*typ.TypeParam]int
	morePrecise MorePreciseFunc
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

type sameExpressionFallbackScan struct {
	seen     sameExpressionFallbackSeen
	maxNodes int
	nodes    int
	exceeded bool
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

func (s *fallbackRefineState) refine(summary, fallback typ.Type) (typ.Type, bool) {
	if summary == nil || fallback == nil || identity.TypeEquals(summary, fallback) || fallbackIsOpaque(fallback) {
		return summary, false
	}
	if s.ownsTypeParam(summary) {
		return summary, false
	}
	if summaryNeedsFallbackLeaf(summary) {
		return fallback, true
	}

	if s.morePrecise != nil && s.morePrecise(summary, fallback) {
		return summary, false
	}

	pair, track := fallbackRefinePair(summary, fallback)
	if track {
		if s.seen == nil {
			s.seen = make(map[typePair]bool)
		}
		if s.seen[pair] {
			return summary, false
		}
		s.seen[pair] = true
		defer delete(s.seen, pair)
	}

	if a, ok := summary.(*typ.Alias); ok {
		refined, changed := s.refine(a.UnaliasedTarget(), fallback)
		if !changed {
			return summary, false
		}
		return typ.NewAlias(a.Name, refined), true
	}
	if b, ok := fallback.(*typ.Alias); ok {
		return s.refine(summary, b.UnaliasedTarget())
	}

	switch a := summary.(type) {
	case *typ.Optional:
		b, ok := fallback.(*typ.Optional)
		if !ok {
			break
		}
		inner, changed := s.refine(a.Inner, b.Inner)
		if !changed {
			return summary, false
		}
		return typ.NewOptional(inner), true
	case *typ.Array:
		b, ok := fallback.(*typ.Array)
		if !ok {
			break
		}
		elem, changed := s.refine(a.Element, b.Element)
		if !changed {
			return summary, false
		}
		return typ.NewArray(elem), true
	case *typ.Map:
		b, ok := fallback.(*typ.Map)
		if !ok {
			break
		}
		key, keyChanged := s.refine(a.Key, b.Key)
		value, valueChanged := s.refine(a.Value, b.Value)
		if !keyChanged && !valueChanged {
			return summary, false
		}
		return typetable.NewMap(key, value), true
	case *typ.ReadonlyMap:
		b, ok := fallback.(*typ.ReadonlyMap)
		if !ok {
			break
		}
		key, keyChanged := s.refine(a.Key, b.Key)
		value, valueChanged := s.refine(a.Value, b.Value)
		if !keyChanged && !valueChanged {
			return summary, false
		}
		return typetable.NewReadonlyMap(key, value), true
	case *typ.Tuple:
		b, ok := fallback.(*typ.Tuple)
		if !ok || len(a.Elements) != len(b.Elements) {
			break
		}
		elements, changed := s.refineTypeSlice(a.Elements, b.Elements)
		if !changed {
			return summary, false
		}
		return typ.NewTuple(elements...), true
	case *typ.Function:
		b, ok := fallback.(*typ.Function)
		if !ok {
			break
		}
		return s.refineFunction(a, b)
	case *typ.Record:
		b, ok := fallback.(*typ.Record)
		if !ok {
			break
		}
		return s.refineRecord(a, b)
	case *typ.Instantiated:
		b, ok := fallback.(*typ.Instantiated)
		if !ok || a.Generic == nil || b.Generic == nil || !identity.TypeEquals(a.Generic, b.Generic) || len(a.TypeArgs) != len(b.TypeArgs) {
			break
		}
		args, changed := s.refineTypeSlice(a.TypeArgs, b.TypeArgs)
		if !changed {
			return summary, false
		}
		return typ.Instantiate(a.Generic, args...), true
	}

	return summary, false
}

func fallbackIsOpaque(t typ.Type) bool {
	return t == nil || typ.AbsentOrUnknown(t) || t.Kind().IsPlaceholder() || ContainsFreeTypeParam(t)
}

func summaryNeedsFallbackLeaf(t typ.Type) bool {
	if t == nil || typ.AbsentOrUnknown(t) || t.Kind().IsPlaceholder() || t.Kind().IsDeferred() {
		return true
	}
	_, ok := t.(*typ.TypeParam)
	return ok
}

func (s *fallbackRefineState) ownsTypeParam(t typ.Type) bool {
	tp, ok := t.(*typ.TypeParam)
	return ok && s.ownedType != nil && s.ownedType[tp] > 0
}

func fallbackRefinePair(summary, fallback typ.Type) (typePair, bool) {
	sp := nodeid.Pointer(summary)
	fp := nodeid.Pointer(fallback)
	if sp == 0 && fp == 0 {
		return typePair{}, false
	}
	return typePair{a: sp, b: fp}, true
}

func (s *fallbackRefineState) refineTypeSlice(summary, fallback []typ.Type) ([]typ.Type, bool) {
	out := make([]typ.Type, len(summary))
	copy(out, summary)
	changed := false
	for i := range summary {
		refined, slotChanged := s.refine(summary[i], fallback[i])
		if slotChanged {
			out[i] = refined
			changed = true
		}
	}
	return out, changed
}

func (s *fallbackRefineState) refineFunction(summary, fallback *typ.Function) (typ.Type, bool) {
	if len(summary.Params) != len(fallback.Params) ||
		len(summary.Returns) != len(fallback.Returns) ||
		(summary.Variadic == nil) != (fallback.Variadic == nil) {
		return summary, false
	}
	s.pushOwnedTypeParams(summary.TypeParams)
	defer s.popOwnedTypeParams(summary.TypeParams)

	params := make([]typ.Param, len(summary.Params))
	copy(params, summary.Params)
	changed := false
	for i := range summary.Params {
		if summary.Params[i].Optional != fallback.Params[i].Optional {
			// Parameter positions are contravariant. A fallback may know a narrower
			// receiver/argument shape than the summary (`self: T` versus the runtime
			// summary's `self: any?`), but tightening that input here would make the
			// callable less permissive. Preserve the summary parameter and keep
			// repairing covariant positions such as returns below.
			continue
		}
		refined, slotChanged := s.refine(summary.Params[i].Type, fallback.Params[i].Type)
		if slotChanged {
			params[i].Type = refined
			changed = true
		}
	}
	returns, returnsChanged := s.refineTypeSlice(summary.Returns, fallback.Returns)
	changed = changed || returnsChanged
	variadic := summary.Variadic
	if summary.Variadic != nil {
		refined, slotChanged := s.refine(summary.Variadic, fallback.Variadic)
		if slotChanged {
			variadic = refined
			changed = true
		}
	}
	if !changed {
		return summary, false
	}
	return typ.RebuildFunction(typ.FunctionParts{
		TypeParams: summary.TypeParams,
		Params:     params,
		Variadic:   variadic,
		Returns:    returns,
	}), true
}

func (s *fallbackRefineState) pushOwnedTypeParams(params []*typ.TypeParam) {
	if len(params) == 0 {
		return
	}
	if s.ownedType == nil {
		s.ownedType = make(map[*typ.TypeParam]int, len(params))
	}
	for _, tp := range params {
		if tp != nil {
			s.ownedType[tp]++
		}
	}
}

func (s *fallbackRefineState) popOwnedTypeParams(params []*typ.TypeParam) {
	if len(params) == 0 || s.ownedType == nil {
		return
	}
	for _, tp := range params {
		if tp == nil {
			continue
		}
		if s.ownedType[tp] <= 1 {
			delete(s.ownedType, tp)
		} else {
			s.ownedType[tp]--
		}
	}
}

func (s *fallbackRefineState) refineRecord(summary, fallback *typ.Record) (typ.Type, bool) {
	fields := make([]typ.Field, len(summary.Fields))
	copy(fields, summary.Fields)
	changed := false
	for i := range fields {
		fb := fallback.GetField(fields[i].Name)
		if fb == nil || fields[i].Optional != fb.Optional {
			continue
		}
		refined, slotChanged := s.refine(fields[i].Type, fb.Type)
		if slotChanged {
			fields[i].Type = refined
			changed = true
		}
	}

	staticMembers := make([]typ.StaticMember, len(summary.StaticMembers))
	copy(staticMembers, summary.StaticMembers)
	for i := range staticMembers {
		fb := fallback.GetStaticMember(staticMembers[i].Kind, staticMembers[i].Name, staticMembers[i].Index)
		if fb == nil || staticMembers[i].Optional != fb.Optional {
			continue
		}
		refined, slotChanged := s.refine(staticMembers[i].Type, fb.Type)
		if slotChanged {
			staticMembers[i].Type = refined
			changed = true
		}
	}

	metatable := summary.Metatable
	if summary.Metatable != nil && fallback.Metatable != nil {
		refined, slotChanged := s.refine(summary.Metatable, fallback.Metatable)
		if slotChanged {
			metatable = refined
			changed = true
		}
	}
	mapKey := summary.MapKey
	if summary.MapKey != nil && fallback.MapKey != nil {
		refined, slotChanged := s.refine(summary.MapKey, fallback.MapKey)
		if slotChanged {
			mapKey = refined
			changed = true
		}
	}
	mapValue := summary.MapValue
	if summary.MapValue != nil && fallback.MapValue != nil {
		refined, slotChanged := s.refine(summary.MapValue, fallback.MapValue)
		if slotChanged {
			mapValue = refined
			changed = true
		}
	}
	if !changed {
		return summary, false
	}
	return typetable.RebuildRecord(typ.RecordParts{
		Fields:        fields,
		StaticMembers: staticMembers,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          summary.Open,
		AssumeSorted:  true,
	}), true
}
