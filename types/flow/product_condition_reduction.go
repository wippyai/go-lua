package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// ProductConditionReduction asks whether a condition fact proves a narrower
// product value for one symbol root.
type ProductConditionReduction struct {
	Symbol   cfg.SymbolID
	Base     product.AbstractValue
	HasBase  bool
	Fact     constraint.Condition
	Facts    PointFacts
	Resolver narrow.Resolver
}

// ProductConditionReductionValue applies the product-domain condition algebra
// for one symbol. Callers own choosing the authoritative base and storing the
// resulting value.
func ProductConditionReductionValue(q ProductConditionReduction) (product.AbstractValue, bool) {
	if q.Symbol == 0 || q.Fact.IsTrue() || !q.Fact.HasConstraints() {
		return product.AbstractValue{}, false
	}
	base := q.Base
	if !q.HasBase || base.IsZero() {
		base = product.Top()
	}
	fact := conditionWithPositiveFieldPresence(q.Fact)
	env, rootKey := SymbolProductEnv(q.Symbol, base, q.Facts, q.Resolver)
	domain := NewProductDomain(env)
	if !domain.ApplyCondition(fact) {
		return product.AbstractValue{}, false
	}
	if !ProductDomainHasNarrowingForSymbol(domain, q.Symbol) {
		return product.AbstractValue{}, false
	}
	projected := domain.ProjectedTypeAt(rootKey, q.Resolver)
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

// SemanticProductReduction reports whether next is a real narrowing of current.
func SemanticProductReduction(current, next product.AbstractValue) bool {
	if current.IsZero() || next.IsZero() {
		return false
	}
	if product.Domain.Equal(current, next) {
		return false
	}
	return current.Covers(next)
}

// ConditionValueSymbols returns all symbol roots mentioned by condition paths.
func ConditionValueSymbols(fact constraint.Condition) []cfg.SymbolID {
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
