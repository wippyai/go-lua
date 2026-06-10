package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// MorePrecise reports whether candidate carries strictly more type information
// than baseline under the gradual-typing precision relation.
//
// Precision is not subtyping: a type can be more precise because it replaces
// any/unknown with concrete structure in the same product shape. This is used
// when two analyses describe the same runtime expression and the checker must
// keep the proof with the most evidence.
func MorePrecise(candidate, baseline Type) bool {
	strict, comparable := ComparePrecision(candidate, baseline)
	return comparable && strict
}

// RefineWithFallback merges two same-expression observations without choosing
// one whole value over the other. The summary observation wins where it already
// carries concrete evidence (for example string literals); the fallback repairs
// only top-like or open type-parameter leaves in the same structural position.
//
// This is intentionally a precision operation, not subtyping. It is used when a
// fixed-point summary and a closed signature/type fallback describe the same
// runtime expression and the summary may still contain internal placeholders
// that should not cross the public/call boundary.
func RefineWithFallback(summary, fallback Type) (Type, bool) {
	state := &fallbackRefineState{}
	out, changed := state.refine(summary, fallback)
	if !changed {
		return summary, false
	}
	return out, true
}

type fallbackRefineState struct {
	seen      map[typePair]bool
	ownedType map[*TypeParam]int
}

// ContainsFreeTypeParam reports whether t contains an unbound symbolic type
// parameter/reference. Unlike ContainsTypeParam, a closed instantiated generic
// such as Box<string> is treated as closed: only its concrete type arguments are
// inspected, not the generic declaration template.
func ContainsFreeTypeParam(t Type) bool {
	return containsFreeTypeParam(t, make(containsSeen), nil)
}

func containsFreeTypeParam(t Type, seen containsSeen, owned map[*TypeParam]int) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}

	switch v := t.(type) {
	case *TypeParam:
		return owned == nil || owned[v] == 0
	}

	switch t.Kind() {
	case kind.Ref, kind.TypeVar, kind.FieldAccess, kind.IndexAccess, kind.Generic:
		return true
	}

	if freeTypeParamUseSeen(owned) {
		if seen.contains(t) {
			return false
		}
		seen.remember(t)
	}

	switch v := t.(type) {
	case *Instantiated:
		for _, arg := range v.TypeArgs {
			if containsFreeTypeParam(arg, seen, owned) {
				return true
			}
		}
		return false
	case *Function:
		nextOwned := owned
		if len(v.TypeParams) > 0 {
			nextOwned = make(map[*TypeParam]int, len(owned)+len(v.TypeParams))
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

	return Visit(t, Visitor[bool]{
		Optional: func(o *Optional) bool {
			return containsFreeTypeParam(o.Inner, seen, owned)
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if containsFreeTypeParam(member, seen, owned) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if containsFreeTypeParam(member, seen, owned) {
					return true
				}
			}
			return false
		},
		Array: func(a *Array) bool {
			return containsFreeTypeParam(a.Element, seen, owned)
		},
		Map: func(m *Map) bool {
			return containsFreeTypeParam(m.Key, seen, owned) || containsFreeTypeParam(m.Value, seen, owned)
		},
		ReadonlyMap: func(m *ReadonlyMap) bool {
			return containsFreeTypeParam(m.Key, seen, owned) || containsFreeTypeParam(m.Value, seen, owned)
		},
		Tuple: func(tup *Tuple) bool {
			for _, elem := range tup.Elements {
				if containsFreeTypeParam(elem, seen, owned) {
					return true
				}
			}
			return false
		},
		Record: func(r *Record) bool {
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
		Alias: func(a *Alias) bool {
			return containsFreeTypeParam(a.Target, seen, owned)
		},
		Meta: func(m *Meta) bool {
			return containsFreeTypeParam(m.Of, seen, owned)
		},
		Recursive: func(r *Recursive) bool {
			return containsFreeTypeParam(r.Body, seen, owned)
		},
		Sum: func(s *Sum) bool {
			for _, variant := range s.Variants {
				for _, t := range variant.Types {
					if containsFreeTypeParam(t, seen, owned) {
						return true
					}
				}
			}
			return false
		},
	})
}

func freeTypeParamUseSeen(owned map[*TypeParam]int) bool {
	return len(owned) == 0
}

// NeedsSameExpressionFallback reports whether t contains a leaf that can be
// repaired by a same-expression fallback. This is deliberately broader than
// free type parameters: a summary return may contain unknown/any/deferred leaves
// inside otherwise precise structure, and those holes should be repaired by a
// closed signature observation without replacing the whole value.
func NeedsSameExpressionFallback(t Type) bool {
	scan := &sameExpressionFallbackScan{seen: make(containsSeen)}
	return scan.needs(t)
}

// NeedsSameExpressionFallbackWithin is the bounded form of
// NeedsSameExpressionFallback. When maxNodes is positive and the scan exceeds
// it, the returned complete flag is false and the caller should treat this as
// "no precision repair from this optional fallback" rather than as proof that no
// repairable leaf exists.
func NeedsSameExpressionFallbackWithin(t Type, maxNodes int) (needs bool, complete bool) {
	scan := &sameExpressionFallbackScan{seen: make(containsSeen), maxNodes: maxNodes}
	needs = scan.needs(t)
	return needs, !scan.exceeded
}

type sameExpressionFallbackScan struct {
	seen     containsSeen
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

func (s *sameExpressionFallbackScan) needs(t Type) bool {
	if !s.enter() {
		return false
	}
	if t == nil {
		return true
	}
	t = UnwrapAnnotated(t)
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
	case *Function:
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
	case *Instantiated:
		for _, arg := range v.TypeArgs {
			if s.needs(arg) {
				return true
			}
		}
		return false
	}

	return Visit(t, Visitor[bool]{
		Optional: func(o *Optional) bool {
			return s.needs(o.Inner)
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if s.needs(member) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if s.needs(member) {
					return true
				}
			}
			return false
		},
		Array: func(a *Array) bool {
			return s.needs(a.Element)
		},
		Map: func(m *Map) bool {
			return s.needs(m.Key) || s.needs(m.Value)
		},
		ReadonlyMap: func(m *ReadonlyMap) bool {
			return s.needs(m.Key) || s.needs(m.Value)
		},
		Tuple: func(tup *Tuple) bool {
			for _, elem := range tup.Elements {
				if s.needs(elem) {
					return true
				}
			}
			return false
		},
		Record: func(r *Record) bool {
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
		Alias: func(a *Alias) bool {
			return s.needs(a.Target)
		},
		Meta: func(m *Meta) bool {
			return s.needs(m.Of)
		},
		Recursive: func(r *Recursive) bool {
			return s.needs(r.Body)
		},
		Sum: func(sum *Sum) bool {
			for _, variant := range sum.Variants {
				for _, t := range variant.Types {
					if s.needs(t) {
						return true
					}
				}
			}
			return false
		},
	})
}

func (s *fallbackRefineState) refine(summary, fallback Type) (Type, bool) {
	if summary == nil || fallback == nil || TypeEquals(summary, fallback) || fallbackIsOpaque(fallback) {
		return summary, false
	}
	if s.ownsTypeParam(summary) {
		return summary, false
	}
	if summaryNeedsFallbackLeaf(summary) {
		return fallback, true
	}

	if MorePrecise(summary, fallback) {
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

	if a, ok := summary.(*Alias); ok {
		refined, changed := s.refine(a.UnaliasedTarget(), fallback)
		if !changed {
			return summary, false
		}
		return NewAlias(a.Name, refined), true
	}
	if b, ok := fallback.(*Alias); ok {
		return s.refine(summary, b.UnaliasedTarget())
	}

	switch a := summary.(type) {
	case *Optional:
		b, ok := fallback.(*Optional)
		if !ok {
			break
		}
		inner, changed := s.refine(a.Inner, b.Inner)
		if !changed {
			return summary, false
		}
		return NewOptional(inner), true
	case *Array:
		b, ok := fallback.(*Array)
		if !ok {
			break
		}
		elem, changed := s.refine(a.Element, b.Element)
		if !changed {
			return summary, false
		}
		return NewArray(elem), true
	case *Map:
		b, ok := fallback.(*Map)
		if !ok {
			break
		}
		key, keyChanged := s.refine(a.Key, b.Key)
		value, valueChanged := s.refine(a.Value, b.Value)
		if !keyChanged && !valueChanged {
			return summary, false
		}
		return NewMap(key, value), true
	case *ReadonlyMap:
		b, ok := fallback.(*ReadonlyMap)
		if !ok {
			break
		}
		key, keyChanged := s.refine(a.Key, b.Key)
		value, valueChanged := s.refine(a.Value, b.Value)
		if !keyChanged && !valueChanged {
			return summary, false
		}
		return NewReadonlyMap(key, value), true
	case *Tuple:
		b, ok := fallback.(*Tuple)
		if !ok || len(a.Elements) != len(b.Elements) {
			break
		}
		elements, changed := s.refineTypeSlice(a.Elements, b.Elements)
		if !changed {
			return summary, false
		}
		return NewTuple(elements...), true
	case *Function:
		b, ok := fallback.(*Function)
		if !ok {
			break
		}
		return s.refineFunction(a, b)
	case *Record:
		b, ok := fallback.(*Record)
		if !ok {
			break
		}
		return s.refineRecord(a, b)
	case *Instantiated:
		b, ok := fallback.(*Instantiated)
		if !ok || a.Generic == nil || b.Generic == nil || !TypeEquals(a.Generic, b.Generic) || len(a.TypeArgs) != len(b.TypeArgs) {
			break
		}
		args, changed := s.refineTypeSlice(a.TypeArgs, b.TypeArgs)
		if !changed {
			return summary, false
		}
		return Instantiate(a.Generic, args...), true
	}

	return summary, false
}

func fallbackIsOpaque(t Type) bool {
	return t == nil || IsAbsentOrUnknown(t) || t.Kind().IsPlaceholder() || ContainsFreeTypeParam(t)
}

func summaryNeedsFallbackLeaf(t Type) bool {
	if t == nil || IsAbsentOrUnknown(t) || t.Kind().IsPlaceholder() || t.Kind().IsDeferred() {
		return true
	}
	_, ok := t.(*TypeParam)
	return ok
}

func (s *fallbackRefineState) ownsTypeParam(t Type) bool {
	tp, ok := t.(*TypeParam)
	return ok && s.ownedType != nil && s.ownedType[tp] > 0
}

func fallbackRefinePair(summary, fallback Type) (typePair, bool) {
	sp := typePointer(summary)
	fp := typePointer(fallback)
	if sp == 0 && fp == 0 {
		return typePair{}, false
	}
	return typePair{a: sp, b: fp}, true
}

func (s *fallbackRefineState) refineTypeSlice(summary, fallback []Type) ([]Type, bool) {
	out := make([]Type, len(summary))
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

func (s *fallbackRefineState) refineFunction(summary, fallback *Function) (Type, bool) {
	if len(summary.Params) != len(fallback.Params) ||
		len(summary.Returns) != len(fallback.Returns) ||
		(summary.Variadic == nil) != (fallback.Variadic == nil) {
		return summary, false
	}
	s.pushOwnedTypeParams(summary.TypeParams)
	defer s.popOwnedTypeParams(summary.TypeParams)

	params := make([]Param, len(summary.Params))
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
	return RebuildFunction(FunctionParts{
		TypeParams: summary.TypeParams,
		Params:     params,
		Variadic:   variadic,
		Returns:    returns,
		Effects:    summary.Effects,
		Spec:       summary.Spec,
		Refinement: summary.Refinement,
	}), true
}

func (s *fallbackRefineState) pushOwnedTypeParams(params []*TypeParam) {
	if len(params) == 0 {
		return
	}
	if s.ownedType == nil {
		s.ownedType = make(map[*TypeParam]int, len(params))
	}
	for _, tp := range params {
		if tp != nil {
			s.ownedType[tp]++
		}
	}
}

func (s *fallbackRefineState) popOwnedTypeParams(params []*TypeParam) {
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

func (s *fallbackRefineState) refineRecord(summary, fallback *Record) (Type, bool) {
	fields := make([]Field, len(summary.Fields))
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

	staticMembers := make([]StaticMember, len(summary.StaticMembers))
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
	return RebuildRecord(RecordParts{
		Fields:        fields,
		StaticMembers: staticMembers,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          summary.Open,
		Fresh:         summary.Fresh,
		AssumeSorted:  true,
	}), true
}

// PruneLessPreciseRefinableUnionMembers removes refinable structural
// placeholder members from a union when another member carries comparable,
// strictly more precise evidence for the same runtime shape.
func PruneLessPreciseRefinableUnionMembers(t Type) Type {
	u, ok := t.(*Union)
	if !ok || len(u.Members) < 2 {
		return t
	}
	keep := make([]Type, 0, len(u.Members))
	for i, member := range u.Members {
		if member == nil {
			continue
		}
		if !IsRefinableAnnotation(member) {
			keep = append(keep, member)
			continue
		}
		dominated := false
		for j, candidate := range u.Members {
			if i == j || candidate == nil {
				continue
			}
			if MorePrecise(candidate, member) {
				dominated = true
				break
			}
		}
		if !dominated {
			keep = append(keep, member)
		}
	}
	if len(keep) == 0 {
		return t
	}
	if len(keep) == len(u.Members) {
		return t
	}
	if len(keep) == 1 {
		return keep[0]
	}
	return NewUnion(keep...)
}

// ComparePrecision compares two same-expression type descriptions.
//
// The first return value is true when candidate is strictly more precise than
// baseline. The second return value is true when the two shapes are comparable
// in the precision relation. Equal types are comparable but not strict.
func ComparePrecision(candidate, baseline Type) (bool, bool) {
	return comparePrecision(candidate, baseline, 0, &precisionSeen{})
}

// productFamilyHash returns a stable structural family hash for product-domain
// relations that must compare recursive products coinductively without
// unfolding them by concrete node identity.
func productFamilyHash(t Type) uint64 {
	return precisionFamilyHash(t, nil)
}

// sameProductFamily reports whether two recursive product observations describe
// the same fixed-point family with equal precision. It is the canonical
// recursive-product equality relation for union/member dedupe and convergence
// checks; generic TypeEquals remains exact structural equality.
func sameProductFamily(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !ContainsRecursive(a) && !ContainsRecursive(b) {
		return false
	}
	if productFamilyHash(a) != productFamilyHash(b) {
		return false
	}
	aStrict, aComparable := ComparePrecision(a, b)
	if !aComparable || aStrict {
		return false
	}
	bStrict, bComparable := ComparePrecision(b, a)
	return bComparable && !bStrict
}

func comparePrecision(candidate, baseline Type, depth int, seen *precisionSeen) (strict bool, comparable bool) {
	if candidate == baseline {
		return false, true
	}
	if baseline == nil {
		return false, false
	}
	if IsAbsentOrUnknown(baseline) || baseline.Kind().IsPlaceholder() {
		return candidate != nil && !IsAbsentOrUnknown(candidate) && !candidate.Kind().IsPlaceholder(), true
	}
	if candidate == nil || IsAbsentOrUnknown(candidate) || candidate.Kind().IsPlaceholder() {
		return false, false
	}
	if candidate.Kind() == kind.Integer && baseline.Kind() == kind.Number {
		return true, true
	}

	if equivalentLocalRefAlias(candidate, baseline) {
		return false, true
	}

	cacheKey, cacheOK := precisionFamilyPairKey(candidate, baseline, seen)
	if cacheOK && seen != nil && seen.results != nil {
		if cached, ok := seen.results[cacheKey]; ok {
			return cached.strict, cached.comparable
		}
	}

	var repeated bool
	var leave func()
	seen, repeated, leave = enterPrecisionPair(candidate, baseline, seen, cacheKey, cacheOK)
	if repeated {
		return false, true
	}
	if leave != nil {
		defer leave()
	}
	if cacheOK && seen != nil {
		defer func() {
			if seen.results == nil {
				seen.results = make(map[precisionFamilyPair]precisionResult)
			}
			seen.results[cacheKey] = precisionResult{strict: strict, comparable: comparable}
		}()
	}

	if c, ok := candidate.(*Alias); ok {
		return comparePrecision(c.UnaliasedTarget(), baseline, depth+1, seen)
	}
	if b, ok := baseline.(*Alias); ok {
		return comparePrecision(candidate, b.UnaliasedTarget(), depth+1, seen)
	}

	if normalized, ok := normalizePrecisionUnion(candidate, seen); ok {
		return comparePrecision(normalized, baseline, depth+1, seen)
	}
	if normalized, ok := normalizePrecisionUnion(baseline, seen); ok {
		return comparePrecision(candidate, normalized, depth+1, seen)
	}

	if precisionCanUseAcyclicEquality(candidate, baseline) && TypeEquals(candidate, baseline) {
		return false, true
	}

	if c, ok := candidate.(*Union); ok {
		if b, ok := baseline.(*Union); ok {
			return compareUnionToUnionPrecision(c, b, depth+1, seen)
		}
	}

	switch b := baseline.(type) {
	case *Union:
		return comparePrecisionAgainstUnion(candidate, b, depth+1, seen)
	case *Recursive:
		if b.Body == nil {
			return false, false
		}
		return comparePrecision(candidate, b.Body, depth+1, seen)
	}

	switch c := candidate.(type) {
	case *Union:
		return compareUnionPrecision(c, baseline, depth+1, seen)
	case *Literal:
		return compareLiteralPrecision(c, baseline)
	case *Record:
		b, ok := baseline.(*Record)
		if !ok {
			return false, false
		}
		return compareRecordPrecision(c, b, depth+1, seen)
	case *Optional:
		b, ok := baseline.(*Optional)
		if !ok {
			return false, false
		}
		return comparePrecision(c.Inner, b.Inner, depth+1, seen)
	case *Tuple:
		b, ok := baseline.(*Tuple)
		if !ok || len(c.Elements) != len(b.Elements) {
			return false, false
		}
		return comparePrecisionSlices(c.Elements, b.Elements, depth+1, seen)
	case *Array:
		b, ok := baseline.(*Array)
		if !ok {
			return false, false
		}
		return comparePrecision(c.Element, b.Element, depth+1, seen)
	case *Map:
		switch b := baseline.(type) {
		case *Map:
			keyStrict, ok := comparePrecision(c.Key, b.Key, depth+1, seen)
			if !ok {
				return false, false
			}
			valueStrict, ok := comparePrecision(c.Value, b.Value, depth+1, seen)
			if !ok {
				return false, false
			}
			return keyStrict || valueStrict, true
		case *Record:
			return compareMapRecordPrecision(c, b, depth+1, seen)
		default:
			return false, false
		}
	case *ReadonlyMap:
		b, ok := baseline.(*ReadonlyMap)
		if !ok {
			return false, false
		}
		keyStrict, ok := comparePrecision(c.Key, b.Key, depth+1, seen)
		if !ok {
			return false, false
		}
		valueStrict, ok := comparePrecision(c.Value, b.Value, depth+1, seen)
		if !ok {
			return false, false
		}
		return keyStrict || valueStrict, true
	case *Instantiated:
		b, ok := baseline.(*Instantiated)
		if !ok || c.Generic == nil || b.Generic == nil || !TypeEquals(c.Generic, b.Generic) || len(c.TypeArgs) != len(b.TypeArgs) {
			return false, false
		}
		return compareGenericTypeArgsPrecision(c.TypeArgs, b.TypeArgs, depth+1, seen)
	case *Function:
		b, ok := baseline.(*Function)
		if !ok || len(c.Params) != len(b.Params) || len(c.Returns) != len(b.Returns) || (c.Variadic == nil) != (b.Variadic == nil) {
			return false, false
		}
		strict := false
		for i := range c.Params {
			if c.Params[i].Optional != b.Params[i].Optional {
				return false, false
			}
			paramStrict, ok := comparePrecision(c.Params[i].Type, b.Params[i].Type, depth+1, seen)
			if !ok {
				return false, false
			}
			strict = strict || paramStrict
		}
		returnStrict, ok := comparePrecisionSlices(c.Returns, b.Returns, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || returnStrict
		if c.Variadic != nil {
			variadicStrict, ok := comparePrecision(c.Variadic, b.Variadic, depth+1, seen)
			if !ok {
				return false, false
			}
			strict = strict || variadicStrict
		}
		return strict, true
	case *Recursive:
		if c.Body == nil {
			return false, false
		}
		if b, ok := baseline.(*Recursive); ok {
			if c.Name != b.Name || b.Body == nil {
				return false, false
			}
			return comparePrecision(c.Body, b.Body, depth+1, seen)
		}
		return comparePrecision(c.Body, baseline, depth+1, seen)
	default:
		return false, false
	}
}

func precisionCanUseAcyclicEquality(candidate, baseline Type) bool {
	return !ContainsRecursive(candidate) && !ContainsRecursive(baseline)
}

func normalizePrecisionUnion(t Type, seen *precisionSeen) (normalized Type, changed bool) {
	u, ok := t.(*Union)
	if !ok || u == nil || len(u.Members) < 2 {
		return nil, false
	}
	if ContainsRecursive(u) {
		return nil, false
	}
	if seen != nil {
		if seen.normalizedUnions != nil {
			if cached, ok := seen.normalizedUnions[t]; ok {
				return cached.typ, cached.changed
			}
		}
		if seen.normalizingUnions != nil && seen.normalizingUnions[t] {
			return nil, false
		}
		if seen.normalizingUnions == nil {
			seen.normalizingUnions = make(map[Type]bool)
		}
		seen.normalizingUnions[t] = true
		defer func() {
			delete(seen.normalizingUnions, t)
			if seen.normalizedUnions == nil {
				seen.normalizedUnions = make(map[Type]precisionUnionNormalization)
			}
			seen.normalizedUnions[t] = precisionUnionNormalization{typ: normalized, changed: changed}
		}()
	}
	members := coalescePrecisionUnionMembers(u.Members)
	if !sameTypeSlice(u.Members, members) {
		return NewUnion(members...), true
	}
	candidate := CoalesceCompatibleRecordAlternatives(u)
	if candidate == nil || candidate == t {
		return nil, false
	}
	return candidate, true
}

func coalescePrecisionUnionMembers(types []Type) []Type {
	state := newReturnJoinState()
	state.recursiveFamilyFold = true
	return state.coalesceProductUnionMembers(types)
}

type precisionSeen struct {
	nodes             map[typePair]int
	families          map[precisionFamilyPair]int
	results           map[precisionFamilyPair]precisionResult
	familyHashes      map[Type]uint64
	normalizedUnions  map[Type]precisionUnionNormalization
	normalizingUnions map[Type]bool
}

type precisionFamilyPair struct {
	candidate uint64
	baseline  uint64
}

type precisionResult struct {
	strict     bool
	comparable bool
}

type precisionUnionNormalization struct {
	typ     Type
	changed bool
}

func enterPrecisionPair(candidate, baseline Type, seen *precisionSeen, familyPair precisionFamilyPair, hasFamilyPair bool) (*precisionSeen, bool, func()) {
	if seen == nil {
		seen = &precisionSeen{}
	}
	releaseNodes := false
	releaseFamilies := false

	cp := typePointer(candidate)
	bp := typePointer(baseline)
	if cp != 0 || bp != 0 {
		if seen.nodes == nil {
			seen.nodes = make(map[typePair]int)
		}
		pair := typePair{a: cp, b: bp}
		if seen.nodes[pair] > 0 {
			return seen, true, nil
		}
		seen.nodes[pair]++
		releaseNodes = true
	}

	if hasFamilyPair {
		if seen.families == nil {
			seen.families = make(map[precisionFamilyPair]int)
		}
		if seen.families[familyPair] > 0 {
			if releaseNodes {
				seen.nodes[typePair{a: cp, b: bp}]--
			}
			return seen, true, nil
		}
		seen.families[familyPair]++
		releaseFamilies = true
	}

	if !releaseNodes && !releaseFamilies {
		return seen, false, nil
	}
	return seen, false, func() {
		if releaseNodes {
			pair := typePair{a: cp, b: bp}
			if seen.nodes[pair] <= 1 {
				delete(seen.nodes, pair)
			} else {
				seen.nodes[pair]--
			}
		}
		if releaseFamilies {
			if seen.families[familyPair] <= 1 {
				delete(seen.families, familyPair)
			} else {
				seen.families[familyPair]--
			}
		}
	}
}

func precisionFamilyPairKey(candidate, baseline Type, seen *precisionSeen) (precisionFamilyPair, bool) {
	if !ContainsRecursive(candidate) && !ContainsRecursive(baseline) {
		return precisionFamilyPair{}, false
	}
	candidateHash := precisionFamilyHash(candidate, seen)
	baselineHash := precisionFamilyHash(baseline, seen)
	return precisionFamilyPair{candidate: candidateHash, baseline: baselineHash}, true
}

func precisionFamilyHash(t Type, seen *precisionSeen) uint64 {
	return precisionFamilyHashSeen(t, make(map[uintptr]bool), seen)
}

func precisionFamilyHashSeen(t Type, active map[uintptr]bool, seen *precisionSeen) (out uint64) {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return 0
	}
	if alias, ok := t.(*Alias); ok {
		return precisionFamilyHashSeen(alias.UnaliasedTarget(), active, seen)
	}
	if rec, ok := t.(*Recursive); ok {
		return hash.HashCombine(uint64(kind.Recursive), hash.FnvString(rec.Name))
	}
	if seen != nil {
		if seen.familyHashes != nil {
			if cached, ok := seen.familyHashes[t]; ok {
				return cached
			}
		}
		defer func() {
			if seen.familyHashes == nil {
				seen.familyHashes = make(map[Type]uint64)
			}
			seen.familyHashes[t] = out
		}()
	}

	ptr := typePointer(t)
	if ptr != 0 {
		if active[ptr] {
			return hash.HashCombine(uint64(kind.Recursive), hash.FnvString("$cycle"))
		}
		active[ptr] = true
		defer delete(active, ptr)
	}

	switch v := t.(type) {
	case *Optional:
		return hash.HashCombine(uint64(kind.Optional), precisionFamilyMemberHash(v.Inner, active, seen))
	case *Union:
		h := hash.HashCombine(uint64(kind.Union), uint64(len(v.Members)))
		for _, member := range v.Members {
			h = hash.HashCombine(h, precisionFamilyMemberHash(member, active, seen))
		}
		return h
	case *Intersection:
		h := hash.HashCombine(uint64(kind.Intersection), uint64(len(v.Members)))
		for _, member := range v.Members {
			h = hash.HashCombine(h, precisionFamilyMemberHash(member, active, seen))
		}
		return h
	case *Record:
		h := hash.HashCombine(uint64(kind.Record), boolPrecisionHash(v.Open))
		h = hash.HashCombine(h, boolPrecisionHash(v.HasMapComponent()))
		if v.HasMapComponent() {
			h = hash.HashCombine(h, precisionFamilyMemberHash(v.MapKey, active, seen))
			h = hash.HashCombine(h, precisionFamilyMemberHash(v.MapValue, active, seen))
		}
		if v.Metatable != nil {
			h = hash.HashCombine(h, precisionFamilyMemberHash(v.Metatable, active, seen))
		}
		h = hash.HashCombine(h, uint64(len(v.Fields)))
		for _, field := range v.Fields {
			h = hash.HashCombine(h, hash.FnvString(field.Name))
			h = hash.HashCombine(h, boolPrecisionHash(field.Optional))
			h = hash.HashCombine(h, boolPrecisionHash(field.Readonly))
			h = hash.HashCombine(h, precisionFamilyTerminalHash(field.Type, seen))
		}
		h = hash.HashCombine(h, uint64(len(v.StaticMembers)))
		for _, member := range v.StaticMembers {
			h = hash.HashCombine(h, uint64(member.Kind))
			h = hash.HashCombine(h, hash.FnvString(member.Name))
			h = hash.HashCombine(h, uint64(member.Index))
			h = hash.HashCombine(h, boolPrecisionHash(member.Optional))
			h = hash.HashCombine(h, boolPrecisionHash(member.Readonly))
			h = hash.HashCombine(h, precisionFamilyTerminalHash(member.Type, seen))
		}
		return h
	case *Array:
		return hash.HashCombine(uint64(kind.Array), precisionFamilyMemberHash(v.Element, active, seen))
	case *Map:
		h := hash.HashCombine(uint64(kind.Map), precisionFamilyMemberHash(v.Key, active, seen))
		return hash.HashCombine(h, precisionFamilyMemberHash(v.Value, active, seen))
	case *ReadonlyMap:
		h := hash.HashCombine(uint64(kind.ReadonlyMap), precisionFamilyMemberHash(v.Key, active, seen))
		return hash.HashCombine(h, precisionFamilyMemberHash(v.Value, active, seen))
	case *Tuple:
		h := hash.HashCombine(uint64(kind.Tuple), uint64(len(v.Elements)))
		for _, elem := range v.Elements {
			h = hash.HashCombine(h, precisionFamilyMemberHash(elem, active, seen))
		}
		return h
	case *Function:
		h := hash.HashCombine(uint64(kind.Function), uint64(len(v.TypeParams)))
		for _, param := range v.TypeParams {
			h = hash.HashCombine(h, precisionFamilyMemberHash(param, active, seen))
		}
		h = hash.HashCombine(h, uint64(len(v.Params)))
		for _, param := range v.Params {
			h = hash.HashCombine(h, boolPrecisionHash(param.Optional))
			h = hash.HashCombine(h, precisionFamilyMemberHash(param.Type, active, seen))
		}
		if v.Variadic != nil {
			h = hash.HashCombine(h, 1)
			h = hash.HashCombine(h, precisionFamilyMemberHash(v.Variadic, active, seen))
		}
		h = hash.HashCombine(h, uint64(len(v.Returns)))
		for _, ret := range v.Returns {
			h = hash.HashCombine(h, precisionFamilyMemberHash(ret, active, seen))
		}
		return h
	case *Instantiated:
		h := hash.HashCombine(uint64(kind.Instantiated), precisionFamilyMemberHash(v.Generic, active, seen))
		for _, arg := range v.TypeArgs {
			h = hash.HashCombine(h, precisionFamilyMemberHash(arg, active, seen))
		}
		return h
	case *Generic:
		h := hash.HashCombine(uint64(kind.Generic), hash.FnvString(v.Name))
		for _, param := range v.TypeParams {
			h = hash.HashCombine(h, precisionFamilyMemberHash(param, active, seen))
		}
		if v.Name == "" && v.Body != nil {
			h = hash.HashCombine(h, precisionFamilyMemberHash(v.Body, active, seen))
		}
		return h
	case *TypeParam:
		h := hash.HashCombine(uint64(kind.TypeParam), hash.FnvString(v.Name))
		if v.Constraint != nil {
			h = hash.HashCombine(h, precisionFamilyMemberHash(v.Constraint, active, seen))
		}
		return h
	case *FieldAccess:
		h := hash.HashCombine(uint64(kind.FieldAccess), precisionFamilyMemberHash(v.Base, active, seen))
		return hash.HashCombine(h, hash.FnvString(v.Field))
	case *IndexAccess:
		h := hash.HashCombine(uint64(kind.IndexAccess), precisionFamilyMemberHash(v.Base, active, seen))
		return hash.HashCombine(h, precisionFamilyMemberHash(v.Index, active, seen))
	case *Meta:
		return hash.HashCombine(uint64(kind.Meta), precisionFamilyMemberHash(v.Of, active, seen))
	case *Sum:
		h := hash.HashCombine(uint64(kind.Sum), hash.FnvString(v.Name))
		for _, variant := range v.Variants {
			h = hash.HashCombine(h, hash.FnvString(variant.Tag))
			for _, vt := range variant.Types {
				h = hash.HashCombine(h, precisionFamilyMemberHash(vt, active, seen))
			}
		}
		return h
	case *Interface:
		h := hash.HashCombine(uint64(kind.Interface), hash.FnvString(v.Name))
		for _, method := range v.Methods {
			h = hash.HashCombine(h, hash.FnvString(method.Name))
			h = hash.HashCombine(h, precisionFamilyMemberHash(method.Type, active, seen))
		}
		return h
	default:
		return t.Hash()
	}
}

func precisionFamilyMemberHash(t Type, active map[uintptr]bool, seen *precisionSeen) (out uint64) {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return 0
	}
	if alias, ok := t.(*Alias); ok {
		return precisionFamilyMemberHash(alias.UnaliasedTarget(), active, seen)
	}
	if rec, ok := t.(*Recursive); ok {
		return hash.HashCombine(uint64(kind.Recursive), hash.FnvString(rec.Name))
	}
	if seen != nil {
		if seen.familyHashes != nil {
			if cached, ok := seen.familyHashes[t]; ok {
				return cached
			}
		}
		defer func() {
			if seen.familyHashes == nil {
				seen.familyHashes = make(map[Type]uint64)
			}
			seen.familyHashes[t] = out
		}()
	}
	if !ContainsRecursive(t) {
		return typeEqualityHash(t)
	}
	return precisionFamilyTerminalHash(t, seen)
}

func precisionFamilyTerminalHash(t Type, seen *precisionSeen) (out uint64) {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return 0
	}
	if alias, ok := t.(*Alias); ok {
		return precisionFamilyTerminalHash(alias.UnaliasedTarget(), seen)
	}
	if rec, ok := t.(*Recursive); ok {
		return hash.HashCombine(uint64(kind.Recursive), hash.FnvString(rec.Name))
	}
	if seen != nil {
		if seen.familyHashes != nil {
			if cached, ok := seen.familyHashes[t]; ok {
				return cached
			}
		}
		defer func() {
			if seen.familyHashes == nil {
				seen.familyHashes = make(map[Type]uint64)
			}
			seen.familyHashes[t] = out
		}()
	}
	if !ContainsRecursive(t) {
		return typeEqualityHash(t)
	}
	return hash.HashCombine(uint64(t.Kind()), hash.FnvString("$recursive-family"))
}

func boolPrecisionHash(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

func comparePrecisionSlices(candidate, baseline []Type, depth int, seen *precisionSeen) (bool, bool) {
	if len(candidate) != len(baseline) {
		return false, false
	}
	strict := false
	for i := range candidate {
		memberStrict, ok := comparePrecision(candidate[i], baseline[i], depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	return strict, true
}

func compareGenericTypeArgsPrecision(candidate, baseline []Type, depth int, seen *precisionSeen) (bool, bool) {
	if len(candidate) != len(baseline) {
		return false, false
	}
	strict := false
	for i := range candidate {
		memberStrict, ok := compareGenericTypeArgPrecision(candidate[i], baseline[i], depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	return strict, true
}

func compareGenericTypeArgPrecision(candidate, baseline Type, depth int, seen *precisionSeen) (bool, bool) {
	if TypeEquals(candidate, baseline) {
		return false, true
	}
	if b, ok := baseline.(*TypeParam); ok {
		if _, ok := candidate.(*TypeParam); ok {
			return false, false
		}
		if b.Constraint == nil {
			return true, true
		}
		_, comparable := comparePrecision(candidate, b.Constraint, depth+1, seen)
		return comparable, comparable
	}
	if _, ok := candidate.(*TypeParam); ok {
		return false, false
	}
	return comparePrecision(candidate, baseline, depth+1, seen)
}

func compareRecordPrecision(candidate, baseline *Record, depth int, seen *precisionSeen) (bool, bool) {
	if candidate == nil || baseline == nil {
		return false, false
	}
	if candidate.Open && !baseline.Open {
		return false, false
	}
	strict := baseline.Open && !candidate.Open

	if baseline.HasMapComponent() {
		if !candidate.HasMapComponent() {
			return false, false
		}
		keyStrict, ok := comparePrecision(candidate.MapKey, baseline.MapKey, depth+1, seen)
		if !ok {
			return false, false
		}
		valueStrict, ok := comparePrecision(candidate.MapValue, baseline.MapValue, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || keyStrict || valueStrict
	} else if candidate.HasMapComponent() {
		strict = true
	}

	// Metatables drive __index field resolution, so records with divergent
	// metatables are not the same product family even when their direct fields
	// match. A baseline without a metatable is comparable to any candidate; a
	// candidate metatable then refines it. When both carry a metatable, compare
	// them structurally so metatable-divergent records stay distinct.
	switch {
	case baseline.Metatable == nil:
		if candidate.Metatable != nil {
			strict = true
		}
	case candidate.Metatable == nil:
		return false, false
	default:
		metaStrict, ok := comparePrecision(candidate.Metatable, baseline.Metatable, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || metaStrict
	}

	for _, baselineField := range baseline.Fields {
		candidateField := candidate.GetField(baselineField.Name)
		if candidateField == nil {
			if baselineField.Optional {
				strict = true
				continue
			}
			return false, false
		}
		if candidateField.Readonly != baselineField.Readonly {
			return false, false
		}
		if candidateField.Optional && !baselineField.Optional {
			return false, false
		}
		if baselineField.Optional && !candidateField.Optional {
			strict = true
		}
		fieldStrict, ok := comparePrecision(candidateField.Type, baselineField.Type, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || fieldStrict
	}
	if len(candidate.Fields) > len(baseline.Fields) {
		strict = true
	}
	for _, baselineMember := range baseline.StaticMembers {
		candidateMember := candidate.GetStaticMember(baselineMember.Kind, baselineMember.Name, baselineMember.Index)
		if candidateMember == nil {
			if baselineMember.Optional {
				strict = true
				continue
			}
			return false, false
		}
		if candidateMember.Readonly != baselineMember.Readonly {
			return false, false
		}
		if candidateMember.Optional && !baselineMember.Optional {
			return false, false
		}
		if baselineMember.Optional && !candidateMember.Optional {
			strict = true
		}
		memberStrict, ok := comparePrecision(candidateMember.Type, baselineMember.Type, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	if len(candidate.StaticMembers) > len(baseline.StaticMembers) {
		strict = true
	}
	return strict, true
}

func compareMapRecordPrecision(candidate *Map, baseline *Record, depth int, seen *precisionSeen) (bool, bool) {
	if candidate == nil || baseline == nil || !baseline.HasMapComponent() || len(baseline.Fields) != 0 || len(baseline.StaticMembers) != 0 || baseline.Metatable != nil {
		return false, false
	}
	keyStrict, ok := comparePrecision(candidate.Key, baseline.MapKey, depth+1, seen)
	if !ok {
		return false, false
	}
	valueStrict, ok := comparePrecision(candidate.Value, baseline.MapValue, depth+1, seen)
	if !ok {
		return false, false
	}
	return baseline.Open || keyStrict || valueStrict, true
}

func compareUnionPrecision(candidate *Union, baseline Type, depth int, seen *precisionSeen) (bool, bool) {
	if candidate == nil || len(candidate.Members) == 0 {
		return false, false
	}
	strict := false
	for _, member := range candidate.Members {
		memberStrict, ok := comparePrecision(member, baseline, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	return strict, true
}

func compareUnionToUnionPrecision(candidate, baseline *Union, depth int, seen *precisionSeen) (bool, bool) {
	if candidate == nil || baseline == nil || len(candidate.Members) == 0 || len(baseline.Members) == 0 {
		return false, false
	}
	strict := len(candidate.Members) < len(baseline.Members)
	for _, member := range candidate.Members {
		memberStrict, ok := comparePrecisionAgainstUnion(member, baseline, depth+1, seen)
		if !ok {
			return false, false
		}
		strict = strict || memberStrict
	}
	return strict, true
}

func comparePrecisionAgainstUnion(candidate Type, baseline *Union, depth int, seen *precisionSeen) (bool, bool) {
	if baseline == nil || len(baseline.Members) == 0 {
		return false, false
	}
	for _, member := range baseline.Members {
		if _, ok := comparePrecision(candidate, member, depth+1, seen); ok {
			return true, true
		}
	}
	return false, false
}

func compareLiteralPrecision(candidate *Literal, baseline Type) (bool, bool) {
	if candidate == nil || baseline == nil {
		return false, false
	}
	switch baseline.Kind() {
	case candidate.Base:
		return true, true
	case kind.Number:
		// Only a numeric literal is in the number family; an integer literal is
		// strictly more precise, a number literal is equal precision. A literal
		// of any other base (e.g. a string literal) is not comparable to number.
		if candidate.Base == kind.Integer || candidate.Base == kind.Number {
			return candidate.Base == kind.Integer, true
		}
		return false, false
	default:
		return false, false
	}
}

func equivalentLocalRefAlias(candidate, baseline Type) bool {
	cRef, cIsRef := candidate.(*Ref)
	bAlias, bIsAlias := baseline.(*Alias)
	if cIsRef && bIsAlias && cRef.Module == "" && cRef.Name == bAlias.Name {
		return true
	}
	cAlias, cIsAlias := candidate.(*Alias)
	bRef, bIsRef := baseline.(*Ref)
	return cIsAlias && bIsRef && bRef.Module == "" && bRef.Name == cAlias.Name
}
