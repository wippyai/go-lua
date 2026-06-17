package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type dynamicIndexFactKey struct {
	table pathdom.PathKey
	site  dynamicindex.Site
}

func normalizeDynamicIndexFacts(reg *axis.Registry, in []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	if len(in) == 0 {
		return nil
	}
	merged := make(map[dynamicIndexFactKey]callboundary.DynamicIndexFact, len(in))
	bottom := dynamicIndexFactBottom(reg)
	for _, fact := range in {
		if !fact.Table.IsPlaceholder() || fact.Site == "" {
			continue
		}
		fact.Table = fact.Table.Clone()
		if dynamicIndexFactEqual(reg, fact, bottom) {
			continue
		}
		key := dynamicIndexKeyOf(fact)
		if existing, ok := merged[key]; ok {
			merged[key] = joinDynamicIndexFact(reg, existing, fact)
			continue
		}
		merged[key] = fact
	}
	return sortedDynamicIndexFacts(merged)
}

func cloneDynamicIndexFacts(in []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.DynamicIndexFact, len(in))
	for i, fact := range in {
		fact.Table = fact.Table.Clone()
		out[i] = fact
	}
	return out
}

func dynamicIndexFactsEqual(reg *axis.Registry, a, b []callboundary.DynamicIndexFact) bool {
	a = normalizeDynamicIndexFacts(reg, a)
	b = normalizeDynamicIndexFacts(reg, b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if dynamicIndexKeyOf(a[i]) != dynamicIndexKeyOf(b[i]) || !dynamicIndexFactEqual(reg, a[i], b[i]) {
			return false
		}
	}
	return true
}

func dynamicIndexFactsLessOrEq(reg *axis.Registry, a, b []callboundary.DynamicIndexFact) bool {
	aMap := dynamicIndexFactsMap(reg, a)
	bMap := dynamicIndexFactsMap(reg, b)
	bottom := dynamicIndexFactBottom(reg)
	for key, av := range aMap {
		bv, ok := bMap[key]
		if !ok {
			bv = bottom
		}
		if !dynamicIndexFactLessOrEq(reg, av, bv) {
			return false
		}
	}
	for key, bv := range bMap {
		if _, ok := aMap[key]; ok {
			continue
		}
		if !dynamicIndexFactLessOrEq(reg, bottom, bv) {
			return false
		}
	}
	return true
}

func joinDynamicIndexFacts(reg *axis.Registry, a, b []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	return combineDynamicIndexMaps(reg, dynamicIndexFactsMap(reg, a), dynamicIndexFactsMap(reg, b), joinDynamicIndexFact)
}

func widenDynamicIndexFacts(reg *axis.Registry, prev, next []callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	return combineDynamicIndexMaps(reg, dynamicIndexFactsMap(reg, prev), dynamicIndexFactsMap(reg, next), widenDynamicIndexFact)
}

func dynamicIndexFactsMap(reg *axis.Registry, in []callboundary.DynamicIndexFact) map[dynamicIndexFactKey]callboundary.DynamicIndexFact {
	out := normalizeDynamicIndexFacts(reg, in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[dynamicIndexFactKey]callboundary.DynamicIndexFact, len(out))
	for _, fact := range out {
		m[dynamicIndexKeyOf(fact)] = fact
	}
	return m
}

func combineDynamicIndexMaps(
	reg *axis.Registry,
	a map[dynamicIndexFactKey]callboundary.DynamicIndexFact,
	b map[dynamicIndexFactKey]callboundary.DynamicIndexFact,
	combine func(*axis.Registry, callboundary.DynamicIndexFact, callboundary.DynamicIndexFact) callboundary.DynamicIndexFact,
) []callboundary.DynamicIndexFact {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[dynamicIndexFactKey]callboundary.DynamicIndexFact, len(a)+len(b))
	for key, left := range a {
		if right, ok := b[key]; ok {
			out[key] = combine(reg, left, right)
			continue
		}
		out[key] = left
	}
	for key, right := range b {
		if _, ok := a[key]; ok {
			continue
		}
		out[key] = right
	}
	return sortedDynamicIndexFacts(out)
}

func dynamicIndexFactBottom(reg *axis.Registry) callboundary.DynamicIndexFact {
	return callboundary.DynamicIndexFact{Value: dynamicindex.Bottom(reg)}
}

func dynamicIndexFactEqual(reg *axis.Registry, a, b callboundary.DynamicIndexFact) bool {
	return dynamicindex.Domain(reg).Equal(a.Value, b.Value)
}

func dynamicIndexFactLessOrEq(reg *axis.Registry, a, b callboundary.DynamicIndexFact) bool {
	return dynamicindex.Domain(reg).LessOrEq(a.Value, b.Value)
}

func joinDynamicIndexFact(reg *axis.Registry, a, b callboundary.DynamicIndexFact) callboundary.DynamicIndexFact {
	return callboundary.DynamicIndexFact{
		Table: a.Table,
		Site:  a.Site,
		Value: dynamicindex.Domain(reg).Join(a.Value, b.Value),
	}
}

func widenDynamicIndexFact(reg *axis.Registry, prev, next callboundary.DynamicIndexFact) callboundary.DynamicIndexFact {
	return callboundary.DynamicIndexFact{
		Table: prev.Table,
		Site:  prev.Site,
		Value: dynamicindex.Domain(reg).Widen(prev.Value, next.Value),
	}
}

func dynamicIndexKeyOf(fact callboundary.DynamicIndexFact) dynamicIndexFactKey {
	return dynamicIndexFactKey{table: fact.Table.Key(), site: fact.Site}
}

func sortedDynamicIndexFacts(in map[dynamicIndexFactKey]callboundary.DynamicIndexFact) []callboundary.DynamicIndexFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.DynamicIndexFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		left := dynamicIndexKeyOf(out[i])
		right := dynamicIndexKeyOf(out[j])
		if left.table != right.table {
			return left.table < right.table
		}
		return left.site < right.site
	})
	return out
}
