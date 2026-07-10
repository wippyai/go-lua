package summary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type pathValueFactKey pathdom.PathKey

// pathValueMap is the canonical may (union) map lattice for path value
// refinements: one product value per path, joined pointwise.
func pathValueMap(reg *axis.Registry) factmap.Map[pathValueFactKey, callboundary.PathValueFact, product.Value] {
	return factmap.Map[pathValueFactKey, callboundary.PathValueFact, product.Value]{
		Key:       func(f callboundary.PathValueFact) pathValueFactKey { return pathValueFactKey(f.Path.Key()) },
		Value:     func(f callboundary.PathValueFact) product.Value { return f.Value },
		WithValue: func(f callboundary.PathValueFact, v product.Value) callboundary.PathValueFact { f.Value = v; return f },
		Less:      func(a, b callboundary.PathValueFact) bool { return a.Path.Less(b.Path) },
		Valid:     func(f callboundary.PathValueFact) bool { return f.Path.IsPlaceholder() },
		CloneFact: func(f callboundary.PathValueFact) callboundary.PathValueFact { f.Path = f.Path.Clone(); return f },
		Domain:    product.Domain(reg),
	}
}

// persistentPathWriteMap is a must map: a persistent write survives a joined
// possible-call summary only when every possible callee writes the same sink.
func persistentPathWriteMap(reg *axis.Registry) factmap.Map[pathValueFactKey, callboundary.PathValueFact, product.Value] {
	return factmap.Map[pathValueFactKey, callboundary.PathValueFact, product.Value]{
		Key:       func(f callboundary.PathValueFact) pathValueFactKey { return pathValueFactKey(f.Path.Key()) },
		Value:     func(f callboundary.PathValueFact) product.Value { return f.Value },
		WithValue: func(f callboundary.PathValueFact, v product.Value) callboundary.PathValueFact { f.Value = v; return f },
		Less:      func(a, b callboundary.PathValueFact) bool { return a.Path.Less(b.Path) },
		Valid: func(f callboundary.PathValueFact) bool {
			return !f.Path.IsEmpty() && !f.Path.IsPlaceholder() && f.Path.Symbol != 0
		},
		CloneFact: func(f callboundary.PathValueFact) callboundary.PathValueFact { f.Path = f.Path.Clone(); return f },
		Domain:    product.Domain(reg),
		Intersect: true,
	}
}

// pathStaticMemberMap is the canonical must (intersection) map lattice for path
// static members: a member survives a join only when guaranteed on every path,
// and value bottom is retained.
func pathStaticMemberMap(reg *axis.Registry) factmap.Map[pathValueFactKey, callboundary.PathStaticMemberFact, product.Value] {
	return factmap.Map[pathValueFactKey, callboundary.PathStaticMemberFact, product.Value]{
		Key:   func(f callboundary.PathStaticMemberFact) pathValueFactKey { return pathValueFactKey(f.Path.Key()) },
		Value: func(f callboundary.PathStaticMemberFact) product.Value { return f.Value },
		WithValue: func(f callboundary.PathStaticMemberFact, v product.Value) callboundary.PathStaticMemberFact {
			f.Value = v
			return f
		},
		Less: func(a, b callboundary.PathStaticMemberFact) bool { return a.Path.Less(b.Path) },
		Valid: func(f callboundary.PathStaticMemberFact) bool {
			return f.Path.IsPlaceholder() || boundaryReturnSlotPath(f.Path) || f.Path.Symbol != 0
		},
		CloneFact: func(f callboundary.PathStaticMemberFact) callboundary.PathStaticMemberFact {
			f.Path = f.Path.Clone()
			return f
		},
		Domain:     product.Domain(reg),
		Intersect:  true,
		KeepBottom: true,
	}
}

func clonePathValueFacts(in []callboundary.PathValueFact) []callboundary.PathValueFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathValueFact, len(in))
	for i, fact := range in {
		fact.Path = fact.Path.Clone()
		out[i] = fact
	}
	return out
}

func clonePathStaticMemberFacts(in []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathStaticMemberFact, len(in))
	for i, fact := range in {
		fact.Path = fact.Path.Clone()
		out[i] = fact
	}
	return out
}

func clonePathStaticMemberDeltaFacts(in []callboundary.PathStaticMemberDeltaFact) []callboundary.PathStaticMemberDeltaFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathStaticMemberDeltaFact, len(in))
	for i, fact := range in {
		fact.Path = fact.Path.Clone()
		out[i] = fact
	}
	return out
}

func normalizePathStaticMemberDeltas(reg *axis.Registry, in []callboundary.PathStaticMemberDeltaFact) []callboundary.PathStaticMemberDeltaFact {
	return normalizePathStaticMemberDeltasWith(reg, in, true)
}

func normalizePathStaticMemberDeltasOwned(reg *axis.Registry, in []callboundary.PathStaticMemberDeltaFact) []callboundary.PathStaticMemberDeltaFact {
	return normalizePathStaticMemberDeltasWith(reg, in, false)
}

func normalizePathStaticMemberDeltasWith(reg *axis.Registry, in []callboundary.PathStaticMemberDeltaFact, clone bool) []callboundary.PathStaticMemberDeltaFact {
	if len(in) == 0 {
		return nil
	}
	domain := product.Domain(reg)
	bottom := domain.Bottom()
	merged := make(map[pathValueFactKey]callboundary.PathStaticMemberDeltaFact, len(in))
	for _, fact := range in {
		if !pathStaticMemberDeltaValid(fact) || domain.Equal(fact.Value, bottom) {
			continue
		}
		if clone {
			fact.Path = fact.Path.Clone()
		}
		key := pathValueFactKey(fact.Path.Key())
		if existing, ok := merged[key]; ok {
			existing.Value = domain.Join(existing.Value, fact.Value)
			existing.Required = existing.Required && fact.Required
			merged[key] = existing
			continue
		}
		merged[key] = fact
	}
	return sortedPathStaticMemberDeltas(merged)
}

func equalPathStaticMemberDeltas(reg *axis.Registry, a, b []callboundary.PathStaticMemberDeltaFact) bool {
	a = normalizePathStaticMemberDeltas(reg, a)
	b = normalizePathStaticMemberDeltas(reg, b)
	if len(a) != len(b) {
		return false
	}
	domain := product.Domain(reg)
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) || a[i].Required != b[i].Required || !domain.Equal(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func lessOrEqPathStaticMemberDeltas(reg *axis.Registry, a, b []callboundary.PathStaticMemberDeltaFact) bool {
	am := pathStaticMemberDeltaFactMap(reg, a)
	bm := pathStaticMemberDeltaFactMap(reg, b)
	domain := product.Domain(reg)
	bottom := domain.Bottom()
	for key, left := range am {
		right, ok := bm[key]
		if !ok {
			right.Value = bottom
		}
		if !domain.LessOrEq(left.Value, right.Value) {
			return false
		}
		if ok && !pathStaticMemberDeltaRequiredLessOrEq(left.Required, right.Required) {
			return false
		}
	}
	for key, right := range bm {
		if _, ok := am[key]; ok {
			continue
		}
		if !domain.LessOrEq(bottom, right.Value) {
			return false
		}
	}
	return true
}

func pathStaticMemberDeltaRequiredLessOrEq(left, right bool) bool {
	return left || !right
}

func joinPathStaticMemberDeltas(reg *axis.Registry, a, b []callboundary.PathStaticMemberDeltaFact) []callboundary.PathStaticMemberDeltaFact {
	return combinePathStaticMemberDeltas(reg, a, b, product.Domain(reg).Join)
}

func widenPathStaticMemberDeltas(reg *axis.Registry, a, b []callboundary.PathStaticMemberDeltaFact) []callboundary.PathStaticMemberDeltaFact {
	return combinePathStaticMemberDeltas(reg, a, b, product.Domain(reg).Widen)
}

func combinePathStaticMemberDeltas(
	reg *axis.Registry,
	a, b []callboundary.PathStaticMemberDeltaFact,
	merge func(product.Value, product.Value) product.Value,
) []callboundary.PathStaticMemberDeltaFact {
	am := pathStaticMemberDeltaFactMap(reg, a)
	bm := pathStaticMemberDeltaFactMap(reg, b)
	out := make(map[pathValueFactKey]callboundary.PathStaticMemberDeltaFact, len(am)+len(bm))
	for key, left := range am {
		if right, ok := bm[key]; ok {
			left.Value = merge(left.Value, right.Value)
			left.Required = left.Required && right.Required
			out[key] = left
			continue
		}
		left.Required = false
		out[key] = left
	}
	for key, right := range bm {
		if _, ok := am[key]; ok {
			continue
		}
		right.Required = false
		out[key] = right
	}
	return sortedPathStaticMemberDeltas(out)
}

func pathStaticMemberDeltaFactMap(reg *axis.Registry, in []callboundary.PathStaticMemberDeltaFact) map[pathValueFactKey]callboundary.PathStaticMemberDeltaFact {
	normalized := normalizePathStaticMemberDeltas(reg, in)
	out := make(map[pathValueFactKey]callboundary.PathStaticMemberDeltaFact, len(normalized))
	for _, fact := range normalized {
		out[pathValueFactKey(fact.Path.Key())] = fact
	}
	return out
}

func pathStaticMemberDeltaValid(f callboundary.PathStaticMemberDeltaFact) bool {
	return f.Path.IsPlaceholder() || boundaryReturnSlotPath(f.Path) || f.Path.Symbol != 0
}

func sortedPathStaticMemberDeltas(in map[pathValueFactKey]callboundary.PathStaticMemberDeltaFact) []callboundary.PathStaticMemberDeltaFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathStaticMemberDeltaFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path.Less(out[j].Path) })
	return out
}
