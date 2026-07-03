package summary

import (
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

func normalizePathValueFacts(reg *axis.Registry, in []callboundary.PathValueFact) []callboundary.PathValueFact {
	return pathValueMap(reg).Normalize(in)
}

func normalizePathValueFactsOwned(reg *axis.Registry, in []callboundary.PathValueFact) []callboundary.PathValueFact {
	return pathValueMap(reg).NormalizeOwned(in)
}

func normalizePersistentPathWrites(reg *axis.Registry, in []callboundary.PathValueFact) []callboundary.PathValueFact {
	return persistentPathWriteMap(reg).Normalize(in)
}

func normalizePersistentPathWritesOwned(reg *axis.Registry, in []callboundary.PathValueFact) []callboundary.PathValueFact {
	return persistentPathWriteMap(reg).NormalizeOwned(in)
}

func normalizePathStaticMemberFacts(reg *axis.Registry, in []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	return pathStaticMemberMap(reg).Normalize(in)
}

func normalizePathStaticMemberFactsOwned(reg *axis.Registry, in []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	return pathStaticMemberMap(reg).NormalizeOwned(in)
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

func pathValueFactsEqual(reg *axis.Registry, a, b []callboundary.PathValueFact) bool {
	return pathValueMap(reg).Equal(a, b)
}

func persistentPathWritesEqual(reg *axis.Registry, a, b []callboundary.PathValueFact) bool {
	return persistentPathWriteMap(reg).Equal(a, b)
}

func pathStaticMemberFactsEqual(reg *axis.Registry, a, b []callboundary.PathStaticMemberFact) bool {
	return pathStaticMemberMap(reg).Equal(a, b)
}

func pathValueFactsLessOrEq(reg *axis.Registry, a, b []callboundary.PathValueFact) bool {
	return pathValueMap(reg).LessOrEq(a, b)
}

func persistentPathWritesLessOrEq(reg *axis.Registry, a, b []callboundary.PathValueFact) bool {
	return persistentPathWriteMap(reg).LessOrEq(a, b)
}

func pathStaticMemberFactsLessOrEq(reg *axis.Registry, a, b []callboundary.PathStaticMemberFact) bool {
	return pathStaticMemberMap(reg).LessOrEq(a, b)
}

func joinPathValueFacts(reg *axis.Registry, a, b []callboundary.PathValueFact) []callboundary.PathValueFact {
	return pathValueMap(reg).Join(a, b)
}

func joinPersistentPathWrites(reg *axis.Registry, a, b []callboundary.PathValueFact) []callboundary.PathValueFact {
	return persistentPathWriteMap(reg).Join(a, b)
}

func widenPathValueFacts(reg *axis.Registry, prev, next []callboundary.PathValueFact) []callboundary.PathValueFact {
	return pathValueMap(reg).Widen(prev, next)
}

func widenPersistentPathWrites(reg *axis.Registry, prev, next []callboundary.PathValueFact) []callboundary.PathValueFact {
	return persistentPathWriteMap(reg).Widen(prev, next)
}

func joinPathStaticMemberFacts(reg *axis.Registry, a, b []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	return pathStaticMemberMap(reg).Join(a, b)
}

func widenPathStaticMemberFacts(reg *axis.Registry, prev, next []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	return pathStaticMemberMap(reg).Widen(prev, next)
}
