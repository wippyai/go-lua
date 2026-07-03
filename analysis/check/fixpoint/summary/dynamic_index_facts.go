package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type dynamicIndexFactKey struct {
	table pathdom.PathKey
	site  dynamicindex.Site
}

// dynamicIndexMap is the canonical pointwise map lattice for dynamic-index
// facts: one fact per (table, site) carrying a dynamicindex value merged through
// the value domain.
func dynamicIndexMap(reg *axis.Registry) factmap.Map[dynamicIndexFactKey, callboundary.DynamicIndexFact, dynamicindex.Fact] {
	return factmap.Map[dynamicIndexFactKey, callboundary.DynamicIndexFact, dynamicindex.Fact]{
		Key:   dynamicIndexKeyOf,
		Value: func(f callboundary.DynamicIndexFact) dynamicindex.Fact { return f.Value },
		WithValue: func(f callboundary.DynamicIndexFact, v dynamicindex.Fact) callboundary.DynamicIndexFact {
			f.Value = v
			return f
		},
		Less:  dynamicIndexFactLess,
		Valid: func(f callboundary.DynamicIndexFact) bool { return boundaryFactPath(f.Table) && f.Site != "" },
		CloneFact: func(f callboundary.DynamicIndexFact) callboundary.DynamicIndexFact {
			f.Table = f.Table.Clone()
			f.KeyPath = f.KeyPath.Clone()
			f.ValuePath = f.ValuePath.Clone()
			return f
		},
		Domain: dynamicindex.Domain(reg),
	}
}

func normalizeDynamicIndexFacts(reg *axis.Registry, in []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	return dynamicIndexMap(reg).Normalize(in)
}

func normalizeDynamicIndexFactsOwned(reg *axis.Registry, in []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	return dynamicIndexMap(reg).NormalizeOwned(in)
}

func cloneDynamicIndexFacts(in []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.DynamicIndexFact, len(in))
	for i, fact := range in {
		fact.Table = fact.Table.Clone()
		fact.KeyPath = fact.KeyPath.Clone()
		fact.ValuePath = fact.ValuePath.Clone()
		out[i] = fact
	}
	return out
}

func dynamicIndexFactsEqual(reg *axis.Registry, a, b []callboundary.DynamicIndexFact) bool {
	return dynamicIndexMap(reg).Equal(a, b)
}

func dynamicIndexFactsLessOrEq(reg *axis.Registry, a, b []callboundary.DynamicIndexFact) bool {
	return dynamicIndexMap(reg).LessOrEq(a, b)
}

func joinDynamicIndexFacts(reg *axis.Registry, a, b []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	return dynamicIndexMap(reg).Join(a, b)
}

func widenDynamicIndexFacts(reg *axis.Registry, prev, next []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	return dynamicIndexMap(reg).Widen(prev, next)
}

func dynamicIndexFactEqual(reg *axis.Registry, a, b callboundary.DynamicIndexFact) bool {
	return dynamicindex.Domain(reg).Equal(a.Value, b.Value)
}

func dynamicIndexKeyOf(fact callboundary.DynamicIndexFact) dynamicIndexFactKey {
	return dynamicIndexFactKey{table: fact.Table.Key(), site: fact.Site}
}

func dynamicIndexFactLess(a, b callboundary.DynamicIndexFact) bool {
	left := dynamicIndexKeyOf(a)
	right := dynamicIndexKeyOf(b)
	if left.table != right.table {
		return left.table < right.table
	}
	return left.site < right.site
}
