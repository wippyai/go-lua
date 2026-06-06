package transfer

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// ConditionEffect is the transfer atom for learning a path-condition fact on the
// current edge. It owns updates to PointState.Cond so guard, assertion, and
// equality-proof code do not hand-edit the condition axis.
type ConditionEffect struct {
	Fact constraint.Condition
}

func (t *Transfer) applyConditionEffect(out *flow.PointState, effect ConditionEffect) bool {
	if out == nil {
		return false
	}
	if effect.Fact.IsFalse() {
		return conditions.assume(out, effect.Fact)
	}
	if !effect.Fact.HasConstraints() {
		return false
	}
	changed := conditions.assume(out, effect.Fact)
	if flow.PointStateDomain.Equal(*out, flow.PointStateDomain.Bottom()) {
		return true
	}
	if t.applyVariantOriginConditionReductions(out, effect.Fact) {
		changed = true
	}
	if t.applyValueConditionReductions(out, effect.Fact) {
		changed = true
	}
	return changed
}

func (t *Transfer) applyValueConditionReductions(out *flow.PointState, fact constraint.Condition) bool {
	if out == nil {
		return false
	}
	syms := conditionValueSymbols(fact)
	if len(syms) == 0 {
		return false
	}
	originSyms := variantOriginConditionSymbolSet(fact)
	changed := false
	for _, sym := range syms {
		if _, hasOriginConstraint := originSyms[sym]; hasOriginConstraint {
			continue
		}
		av, hasValue := t.symbolValue(out, sym)
		base, hasBase := t.narrowBase(sym, av, false)
		if !hasBase && t.unannotatedParam[sym] && (!hasValue || av.IsZero()) {
			// Parameter contracts are co-solved with the body. Until an unannotated
			// parameter has a declared or entry-projected value to reduce, keep the
			// proof in Cond rather than materializing a broad value fact into Env.
			continue
		}
		if !hasBase {
			base = av
			hasBase = hasValue
		}
		next, narrowed := t.conditionProductReductionValue(out, sym, base, hasBase, fact)
		if !narrowed || next.IsZero() {
			continue
		}
		if hasValue && !av.IsZero() {
			if !semanticProductReduction(av, next) {
				continue
			}
		} else if !base.IsZero() && !semanticProductReduction(base, next) {
			continue
		}
		t.applyRefinementEffect(out, RefinementEffect{
			Place: Place{Root: sym},
			Kind:  RefinementSetValue,
			Value: next,
		})
		changed = true
	}
	return changed
}

func semanticProductReduction(current, next product.AbstractValue) bool {
	if current.IsZero() || next.IsZero() {
		return false
	}
	if product.Domain.Equal(current, next) {
		return false
	}
	return current.Covers(next)
}

func (t *Transfer) conditionProductReductionValue(
	out *flow.PointState,
	sym cfg.SymbolID,
	base product.AbstractValue,
	hasBase bool,
	fact constraint.Condition,
) (product.AbstractValue, bool) {
	if out == nil || sym == 0 || fact.IsTrue() || !fact.HasConstraints() {
		return product.AbstractValue{}, false
	}
	if !hasBase || base.IsZero() {
		base = product.Top()
	}
	fact = conditionWithPositiveFieldPresence(fact)
	rootKey := flow.SymbolPathKey(sym, nil)
	pathTypeCache := make(map[constraint.PathKey]typ.Type)
	env := constraint.Env{
		Resolver: fieldResolver,
		ResolvePath: func(path constraint.Path) constraint.PathKey {
			return flow.StablePathKey(path)
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			if cached, ok := pathTypeCache[key]; ok {
				return cached
			}
			keySym, segments, ok := flow.ParseSymbolPathKey(key)
			if !ok || keySym != sym {
				return nil
			}
			if len(segments) == 0 {
				t := product.ProjectValueOrUnknown(base)
				pathTypeCache[key] = t
				return t
			}
			if t, ok := productPathType(base, segments); ok {
				pathTypeCache[key] = t
				return t
			}
			path := constraint.Path{Symbol: sym, Segments: append([]constraint.Segment(nil), segments...)}
			if t, ok := flow.PointFactsOf(*out).PathType(path); ok {
				pathTypeCache[key] = t
				return t
			}
			pathTypeCache[key] = nil
			return nil
		},
	}
	domain := flow.NewProductDomain(env)
	if !domain.ApplyCondition(fact) {
		return product.AbstractValue{}, false
	}
	if !productDomainHasSymbolNarrowing(domain, sym) {
		return product.AbstractValue{}, false
	}
	projected := domain.ProjectedTypeAt(rootKey, fieldResolver)
	if typ.IsAbsentOrUnknown(projected) {
		return product.AbstractValue{}, false
	}
	if typ.IsNever(projected) {
		return product.Bottom(), true
	}
	baseType := product.ProjectValueOrUnknown(base)
	if typ.SameNode(projected, baseType) || typ.SameNodeOrAcyclicEqual(projected, baseType) {
		return product.AbstractValue{}, false
	}
	return product.FromRefinedType(base, projected), true
}

func productDomainHasSymbolNarrowing(domain *flow.ProductDomain, sym cfg.SymbolID) bool {
	if domain == nil || sym == 0 {
		return false
	}
	for key := range domain.Type.Narrowed {
		if productDomainKeyBelongsToSymbol(key, sym) {
			return true
		}
	}
	for key := range domain.Shape.Narrowed {
		if productDomainKeyBelongsToSymbol(key, sym) {
			return true
		}
	}
	return false
}

func productDomainKeyBelongsToSymbol(key constraint.PathKey, sym cfg.SymbolID) bool {
	keySym, _, ok := flow.ParseSymbolPathKey(key)
	return ok && keySym == sym
}

func conditionWithPositiveFieldPresence(fact constraint.Condition) constraint.Condition {
	if fact.NumDisjuncts() == 0 || !fact.HasConstraints() {
		return fact
	}
	conjunctions := make([][]constraint.Constraint, 0, fact.NumDisjuncts())
	changed := false
	for i := 0; i < fact.NumDisjuncts(); i++ {
		disjunct := fact.DisjunctConstraints(i)
		next := append([]constraint.Constraint(nil), disjunct...)
		for _, c := range disjunct {
			before := len(next)
			switch cc := c.(type) {
			case constraint.Truthy:
				next = appendPositiveFieldPresence(next, cc.Path)
			case constraint.NotNil:
				next = appendPositiveFieldPresence(next, cc.Path)
			case constraint.HasType:
				if hasTypeImpliesPresentPath(cc) {
					next = appendPositiveFieldPresence(next, cc.Path)
				}
			case constraint.FieldEquals:
				if cc.Value != nil && cc.Field != "" {
					next = appendPositiveFieldPresence(next, cc.Target.Field(cc.Field))
				}
			}
			if len(next) != before {
				changed = true
			}
		}
		conjunctions = append(conjunctions, next)
	}
	if !changed {
		return fact
	}
	return constraint.FromDisjuncts(conjunctions)
}

func hasTypeImpliesPresentPath(c constraint.HasType) bool {
	if c.Type.IsZero() {
		return false
	}
	if k, ok := c.Type.BuiltinKind(); ok && k == kind.Nil {
		return false
	}
	return true
}

func appendPositiveFieldPresence(out []constraint.Constraint, path constraint.Path) []constraint.Constraint {
	if path.IsEmpty() || len(path.Segments) == 0 {
		return out
	}
	parent := constraint.Path{
		Root:    path.Root,
		Symbol:  path.Symbol,
		Version: path.Version,
	}
	for _, seg := range path.Segments {
		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if seg.Name != "" {
				out = append(out, constraint.HasField{
					Path:  parent,
					Field: seg.Name,
				})
			}
		}
		parent = parent.Append(seg)
	}
	return out
}

func productPathType(base product.AbstractValue, segments []constraint.Segment) (typ.Type, bool) {
	if base.IsZero() {
		return nil, false
	}
	t := product.ProjectValueOrUnknown(base)
	if typ.IsAbsentOrUnknown(t) {
		return nil, false
	}
	if len(segments) == 0 {
		return t, true
	}
	return readFieldPath(t, segments)
}

func (t *Transfer) applyVariantOriginConditionReductions(out *flow.PointState, fact constraint.Condition) bool {
	if out == nil {
		return false
	}
	syms := variantOriginConditionSymbols(fact)
	if len(syms) == 0 {
		return false
	}
	changed := false
	for _, sym := range syms {
		av, ok := t.symbolValue(out, sym)
		if !ok || av.IsZero() {
			continue
		}
		next, narrowed := variantOriginValueForCondition(av, sym, fact)
		if !narrowed || product.Domain.Equal(av, next) {
			continue
		}
		t.setSymbolValue(out, sym, next, false)
		changed = true
	}
	return changed
}

func variantOriginValueForCondition(av product.AbstractValue, sym cfg.SymbolID, fact constraint.Condition) (product.AbstractValue, bool) {
	if fact.NumDisjuncts() == 0 {
		return product.AbstractValue{}, false
	}
	var joined product.AbstractValue
	joinedSet := false
	for i := 0; i < fact.NumDisjuncts(); i++ {
		candidate := av
		for _, c := range fact.DisjunctConstraints(i) {
			next, ok := variantOriginValueForConstraint(candidate, sym, c)
			if ok {
				candidate = next
			}
		}
		if !joinedSet {
			joined = candidate
			joinedSet = true
			continue
		}
		joined = product.Domain.Join(joined, candidate)
	}
	if !joinedSet {
		return product.AbstractValue{}, false
	}
	return joined, true
}

func variantOriginValueForConstraint(av product.AbstractValue, sym cfg.SymbolID, c constraint.Constraint) (product.AbstractValue, bool) {
	switch cc := c.(type) {
	case constraint.VariantCaseEquals:
		return variantOriginValueForCase(av, sym, cc.Target, cc.OriginFamily, cc.CaseIndex, true)
	case constraint.VariantCaseNotEquals:
		return variantOriginValueForCase(av, sym, cc.Target, cc.OriginFamily, cc.CaseIndex, false)
	default:
		return product.AbstractValue{}, false
	}
}

func variantOriginValueForCase(av product.AbstractValue, sym cfg.SymbolID, path constraint.Path, family uint64, caseIndex int, equal bool) (product.AbstractValue, bool) {
	if path.Symbol != sym || len(path.Segments) != 0 {
		return product.AbstractValue{}, false
	}
	return product.NarrowVariantOriginCase(av, family, caseIndex, equal)
}

func variantOriginConditionSymbols(fact constraint.Condition) []cfg.SymbolID {
	seen := variantOriginConditionSymbolSet(fact)
	if len(seen) == 0 {
		return nil
	}
	out := make([]cfg.SymbolID, 0, len(seen))
	for sym := range seen {
		out = append(out, sym)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func variantOriginConditionSymbolSet(fact constraint.Condition) map[cfg.SymbolID]struct{} {
	seen := make(map[cfg.SymbolID]struct{})
	for i := 0; i < fact.NumDisjuncts(); i++ {
		for _, c := range fact.DisjunctConstraints(i) {
			switch cc := c.(type) {
			case constraint.VariantCaseEquals:
				if cc.Target.Symbol != 0 && len(cc.Target.Segments) == 0 {
					seen[cc.Target.Symbol] = struct{}{}
				}
			case constraint.VariantCaseNotEquals:
				if cc.Target.Symbol != 0 && len(cc.Target.Segments) == 0 {
					seen[cc.Target.Symbol] = struct{}{}
				}
			}
		}
	}
	return seen
}

func conditionValueSymbols(fact constraint.Condition) []cfg.SymbolID {
	seen := make(map[cfg.SymbolID]struct{})
	for i := 0; i < fact.NumDisjuncts(); i++ {
		for _, c := range fact.DisjunctConstraints(i) {
			constraint.VisitPaths(c, func(path constraint.Path) bool {
				if path.Symbol != 0 {
					seen[path.Symbol] = struct{}{}
				}
				return false
			})
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]cfg.SymbolID, 0, len(seen))
	for sym := range seen {
		out = append(out, sym)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
