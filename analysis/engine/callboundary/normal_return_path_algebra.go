package callboundary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type pathValueFactKey pathdom.PathKey

// pathValueMap is the canonical may (union) map lattice for path value
// refinements: one product value per path, joined pointwise.
func pathValueMap(reg *axis.Registry) factmap.Map[pathValueFactKey, PathValueFact, product.Value] {
	return factmap.Map[pathValueFactKey, PathValueFact, product.Value]{
		Key:       func(f PathValueFact) pathValueFactKey { return pathValueFactKey(f.Path.Key()) },
		Value:     func(f PathValueFact) product.Value { return f.Value },
		WithValue: func(f PathValueFact, v product.Value) PathValueFact { f.Value = v; return f },
		Less:      func(a, b PathValueFact) bool { return a.Path.Less(b.Path) },
		Valid:     func(f PathValueFact) bool { return f.Path.IsPlaceholder() },
		CloneFact: func(f PathValueFact) PathValueFact { f.Path = f.Path.Clone(); return f },
		Domain:    product.Domain(reg),
	}
}

// persistentPathWriteMap is a must map: a persistent write survives a joined
// possible-call summary only when every possible callee writes the same sink.
func persistentPathWriteMap(reg *axis.Registry) factmap.Map[pathValueFactKey, PathValueFact, product.Value] {
	return factmap.Map[pathValueFactKey, PathValueFact, product.Value]{
		Key:       func(f PathValueFact) pathValueFactKey { return pathValueFactKey(f.Path.Key()) },
		Value:     func(f PathValueFact) product.Value { return f.Value },
		WithValue: func(f PathValueFact, v product.Value) PathValueFact { f.Value = v; return f },
		Less:      func(a, b PathValueFact) bool { return a.Path.Less(b.Path) },
		Valid: func(f PathValueFact) bool {
			return !f.Path.IsEmpty() && !f.Path.IsPlaceholder() && f.Path.Symbol != 0
		},
		CloneFact: func(f PathValueFact) PathValueFact { f.Path = f.Path.Clone(); return f },
		Domain:    product.Domain(reg),
		Intersect: true,
	}
}

// pathStaticMemberMap is the canonical must (intersection) map lattice for path
// static members: a member survives a join only when guaranteed on every path,
// and value bottom is retained.
func pathStaticMemberMap(reg *axis.Registry) factmap.Map[pathValueFactKey, PathStaticMemberFact, product.Value] {
	return factmap.Map[pathValueFactKey, PathStaticMemberFact, product.Value]{
		Key:   func(f PathStaticMemberFact) pathValueFactKey { return pathValueFactKey(f.Path.Key()) },
		Value: func(f PathStaticMemberFact) product.Value { return f.Value },
		WithValue: func(f PathStaticMemberFact, v product.Value) PathStaticMemberFact {
			f.Value = v
			return f
		},
		Less: func(a, b PathStaticMemberFact) bool { return a.Path.Less(b.Path) },
		Valid: func(f PathStaticMemberFact) bool {
			return f.Path.IsPlaceholder() || boundaryReturnSlotPath(f.Path) || f.Path.Symbol != 0
		},
		CloneFact: func(f PathStaticMemberFact) PathStaticMemberFact {
			f.Path = f.Path.Clone()
			return f
		},
		Domain:     product.Domain(reg),
		Intersect:  true,
		KeepBottom: true,
	}
}

// pathStaticMemberDeltaMap is the canonical may map for structural param
// additions. Unlike PathStaticMembers, it keeps branch-local writes so consumers
// can materialize them as optional members.
func pathStaticMemberDeltaMap(reg *axis.Registry) factmap.Map[pathValueFactKey, PathStaticMemberDeltaFact, product.Value] {
	return factmap.Map[pathValueFactKey, PathStaticMemberDeltaFact, product.Value]{
		Key:   func(f PathStaticMemberDeltaFact) pathValueFactKey { return pathValueFactKey(f.Path.Key()) },
		Value: func(f PathStaticMemberDeltaFact) product.Value { return f.Value },
		WithValue: func(f PathStaticMemberDeltaFact, v product.Value) PathStaticMemberDeltaFact {
			f.Value = v
			return f
		},
		Less: func(a, b PathStaticMemberDeltaFact) bool { return a.Path.Less(b.Path) },
		Valid: func(f PathStaticMemberDeltaFact) bool {
			return f.Path.IsPlaceholder() || boundaryReturnSlotPath(f.Path) || f.Path.Symbol != 0
		},
		CloneFact: func(f PathStaticMemberDeltaFact) PathStaticMemberDeltaFact {
			f.Path = f.Path.Clone()
			return f
		},
		Domain: product.Domain(reg),
	}
}

func clonePathValueFacts(in []PathValueFact) []PathValueFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathValueFact, len(in))
	for i, fact := range in {
		fact.Path = fact.Path.Clone()
		out[i] = fact
	}
	return out
}

func clonePathStaticMemberFacts(in []PathStaticMemberFact) []PathStaticMemberFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathStaticMemberFact, len(in))
	for i, fact := range in {
		fact.Path = fact.Path.Clone()
		out[i] = fact
	}
	return out
}

func clonePathStaticMemberDeltaFacts(in []PathStaticMemberDeltaFact) []PathStaticMemberDeltaFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathStaticMemberDeltaFact, len(in))
	for i, fact := range in {
		fact.Path = fact.Path.Clone()
		out[i] = fact
	}
	return out
}

func normalizePathStaticMemberDeltas(reg *axis.Registry, in []PathStaticMemberDeltaFact) []PathStaticMemberDeltaFact {
	return normalizePathStaticMemberDeltasWith(reg, in, true)
}

func normalizePathStaticMemberDeltasOwned(reg *axis.Registry, in []PathStaticMemberDeltaFact) []PathStaticMemberDeltaFact {
	return normalizePathStaticMemberDeltasWith(reg, in, false)
}

func normalizePathStaticMemberDeltasWith(reg *axis.Registry, in []PathStaticMemberDeltaFact, clone bool) []PathStaticMemberDeltaFact {
	if len(in) == 0 {
		return nil
	}
	domain := product.Domain(reg)
	bottom := domain.Bottom()
	merged := make(map[pathValueFactKey]PathStaticMemberDeltaFact, len(in))
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

func equalPathStaticMemberDeltas(reg *axis.Registry, a, b []PathStaticMemberDeltaFact) bool {
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

func lessOrEqPathStaticMemberDeltas(reg *axis.Registry, a, b []PathStaticMemberDeltaFact) bool {
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

func joinPathStaticMemberDeltas(reg *axis.Registry, a, b []PathStaticMemberDeltaFact) []PathStaticMemberDeltaFact {
	return combinePathStaticMemberDeltas(reg, a, b, product.Domain(reg).Join)
}

func widenPathStaticMemberDeltas(reg *axis.Registry, a, b []PathStaticMemberDeltaFact) []PathStaticMemberDeltaFact {
	return combinePathStaticMemberDeltas(reg, a, b, product.Domain(reg).Widen)
}

func combinePathStaticMemberDeltas(
	reg *axis.Registry,
	a, b []PathStaticMemberDeltaFact,
	merge func(product.Value, product.Value) product.Value,
) []PathStaticMemberDeltaFact {
	am := pathStaticMemberDeltaFactMap(reg, a)
	bm := pathStaticMemberDeltaFactMap(reg, b)
	out := make(map[pathValueFactKey]PathStaticMemberDeltaFact, len(am)+len(bm))
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

func pathStaticMemberDeltaFactMap(reg *axis.Registry, in []PathStaticMemberDeltaFact) map[pathValueFactKey]PathStaticMemberDeltaFact {
	normalized := normalizePathStaticMemberDeltas(reg, in)
	out := make(map[pathValueFactKey]PathStaticMemberDeltaFact, len(normalized))
	for _, fact := range normalized {
		out[pathValueFactKey(fact.Path.Key())] = fact
	}
	return out
}

func pathStaticMemberDeltaValid(f PathStaticMemberDeltaFact) bool {
	return f.Path.IsPlaceholder() || boundaryReturnSlotPath(f.Path) || f.Path.Symbol != 0
}

func sortedPathStaticMemberDeltas(in map[pathValueFactKey]PathStaticMemberDeltaFact) []PathStaticMemberDeltaFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathStaticMemberDeltaFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path.Less(out[j].Path) })
	return out
}
