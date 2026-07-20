package callboundary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type dynamicIndexFactKey struct {
	table pathdom.PathKey
	site  dynamicindex.Site
}

// dynamicIndexMap is the canonical pointwise map lattice for dynamic-index
// facts: one fact per (table, site) carrying a dynamicindex value merged through
// the value domain.
func dynamicIndexMap(reg *axis.Registry) factmap.Map[dynamicIndexFactKey, DynamicIndexFact, dynamicindex.Fact] {
	return factmap.Map[dynamicIndexFactKey, DynamicIndexFact, dynamicindex.Fact]{
		Key:   dynamicIndexKeyOf,
		Value: func(f DynamicIndexFact) dynamicindex.Fact { return f.Value },
		WithValue: func(f DynamicIndexFact, v dynamicindex.Fact) DynamicIndexFact {
			f.Value = v
			return f
		},
		Less:  dynamicIndexFactLess,
		Valid: func(f DynamicIndexFact) bool { return boundaryFactPath(f.Table) && f.Site != "" },
		CloneFact: func(f DynamicIndexFact) DynamicIndexFact {
			f.Table = f.Table.Clone()
			f.KeyPath = f.KeyPath.Clone()
			f.ValuePath = f.ValuePath.Clone()
			return f
		},
		Domain: dynamicindex.Domain(reg),
	}
}

func cloneDynamicIndexFacts(in []DynamicIndexFact) []DynamicIndexFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]DynamicIndexFact, len(in))
	for i, fact := range in {
		fact.Table = fact.Table.Clone()
		fact.KeyPath = fact.KeyPath.Clone()
		fact.ValuePath = fact.ValuePath.Clone()
		out[i] = fact
	}
	return out
}

func dynamicIndexFactEqual(reg *axis.Registry, a, b DynamicIndexFact) bool {
	return dynamicindex.Domain(reg).Equal(a.Value, b.Value)
}

func dynamicIndexKeyOf(fact DynamicIndexFact) dynamicIndexFactKey {
	return dynamicIndexFactKey{table: fact.Table.Key(), site: fact.Site}
}

func dynamicIndexFactLess(a, b DynamicIndexFact) bool {
	left := dynamicIndexKeyOf(a)
	right := dynamicIndexKeyOf(b)
	if left.table != right.table {
		return left.table < right.table
	}
	return left.site < right.site
}
