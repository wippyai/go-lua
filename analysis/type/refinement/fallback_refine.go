package refinement

import (
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type typePair struct {
	a uintptr
	b uintptr
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

func (s *fallbackRefineState) refine(summary, fallback typ.Type) (typ.Type, bool) {
	if summary == nil || fallback == nil || typ.TypeEquals(summary, fallback) || fallbackIsOpaque(fallback) {
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
		if !ok || a.Generic == nil || b.Generic == nil || !typ.TypeEquals(a.Generic, b.Generic) || len(a.TypeArgs) != len(b.TypeArgs) {
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
